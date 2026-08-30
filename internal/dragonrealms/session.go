package dragonrealms

import (
	"context"
	"errors"
	"io"
	"net"
	"strings"
	"sync"
	"time"
	"unicode/utf8"
)

const (
	gameFrontendLine = "FE:GENIE /VERSION:5.0.0.1 /P:WIN_UNKNOWN /XML\n"
	updateCapacity   = 256
)

type sessionOptions struct {
	authenticate    func(context.Context, Credentials) (sgeEntry, error)
	dialGame        func(context.Context, string, string) (net.Conn, error)
	wait            func(context.Context, time.Duration) error
	initialAttempts int
	initialDelay    time.Duration
	attemptTimeout  time.Duration
	reconnectDelays []time.Duration
	watchdog        time.Duration
	updateCapacity  int
}

func defaultSessionOptions() sessionOptions {
	sgeConfig := defaultSGEDialConfig()
	dialer := &net.Dialer{Timeout: 20 * time.Second, KeepAlive: 60 * time.Second}
	return sessionOptions{
		authenticate: func(ctx context.Context, credentials Credentials) (sgeEntry, error) {
			return authenticateSGE(ctx, credentials, sgeConfig)
		},
		dialGame:        dialer.DialContext,
		wait:            waitContext,
		initialAttempts: 11,
		initialDelay:    5 * time.Second,
		attemptTimeout:  20 * time.Second,
		reconnectDelays: []time.Duration{8 * time.Second, 8 * time.Second, 30 * time.Second, 30 * time.Second, 30 * time.Second},
		watchdog:        5 * time.Minute,
		updateCapacity:  updateCapacity,
	}
}

// Session owns one DragonRealms connection and its protocol state.
type Session struct {
	credentials Credentials
	options     sessionOptions

	ctx    context.Context
	cancel context.CancelFunc

	mu             sync.RWMutex
	conn           net.Conn
	state          ConnectionState
	reconnectArmed bool
	callerClosing  bool

	writeMu   sync.Mutex
	updates   chan Update
	done      chan struct{}
	wg        sync.WaitGroup
	closeOnce sync.Once
}

// Dial authenticates and opens one DragonRealms session.
func Dial(ctx context.Context, credentials Credentials) (*Session, error) {
	return dialWithOptions(ctx, credentials, defaultSessionOptions())
}

func dialWithOptions(parent context.Context, credentials Credentials, options sessionOptions) (*Session, error) {
	if err := validateCredentials(credentials); err != nil {
		return nil, err
	}
	if options.initialAttempts < 1 {
		options.initialAttempts = 1
	}
	if options.updateCapacity < 1 {
		options.updateCapacity = updateCapacity
	}

	var conn net.Conn
	var err error
	for attempt := 0; attempt < options.initialAttempts; attempt++ {
		attemptCtx, cancel := context.WithTimeout(parent, options.attemptTimeout)
		conn, err = connectGame(attemptCtx, credentials, options)
		attemptErr := attemptCtx.Err()
		cancel()
		if err == nil {
			break
		}
		var authErr *sgeAuthError
		if errors.As(err, &authErr) {
			return nil, sanitizedConnectError(err)
		}
		if parentErr := parent.Err(); parentErr != nil {
			return nil, sanitizedConnectError(parentErr)
		}
		if errors.Is(attemptErr, context.DeadlineExceeded) || errors.Is(err, context.DeadlineExceeded) {
			return nil, sanitizedConnectError(context.DeadlineExceeded)
		}
		if attempt+1 < options.initialAttempts {
			if waitErr := options.wait(parent, options.initialDelay); waitErr != nil {
				return nil, sanitizedConnectError(waitErr)
			}
		}
	}
	if err != nil {
		return nil, sanitizedConnectError(err)
	}

	ctx, cancel := context.WithCancel(parent)
	session := &Session{
		credentials: credentials,
		options:     options,
		ctx:         ctx,
		cancel:      cancel,
		conn:        conn,
		state:       ConnectionConnected,
		updates:     make(chan Update, options.updateCapacity),
		done:        make(chan struct{}),
	}
	session.wg.Add(2)
	go session.watchCancellation()
	go session.readLoop()
	go func() {
		session.wg.Wait()
		close(session.updates)
		close(session.done)
	}()
	return session, nil
}

func connectGame(ctx context.Context, credentials Credentials, options sessionOptions) (net.Conn, error) {
	entry, err := options.authenticate(ctx, credentials)
	if err != nil {
		return nil, err
	}
	conn, err := options.dialGame(ctx, "tcp", formatGameAddress(entry))
	if err != nil {
		return nil, err
	}
	stopClose := context.AfterFunc(ctx, func() {
		_ = conn.Close()
	})
	fail := func(message string) (net.Conn, error) {
		stopClose()
		_ = conn.Close()
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New(message)
	}
	if tcp, ok := conn.(*net.TCPConn); ok {
		_ = tcp.SetKeepAliveConfig(net.KeepAliveConfig{
			Enable:   true,
			Idle:     60 * time.Second,
			Interval: 10 * time.Second,
			Count:    5,
		})
	}
	if err := writeAll(conn, []byte(entry.key+"\n")); err != nil {
		return fail("could not send the DragonRealms game key")
	}
	if err := writeAll(conn, []byte(gameFrontendLine)); err != nil {
		return fail("could not identify the DragonRealms frontend")
	}
	if !stopClose() {
		_ = conn.Close()
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		return nil, errors.New("DragonRealms game handshake was interrupted")
	}
	return conn, nil
}

// Send writes one player command with DragonRealms CRLF framing.
func (s *Session) Send(command string) error {
	if !utf8.ValidString(command) || strings.ContainsAny(command, "\r\n\x00") {
		return ErrInvalidCommand
	}

	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	s.mu.RLock()
	state := s.state
	conn := s.conn
	s.mu.RUnlock()
	switch state {
	case ConnectionReconnecting:
		return ErrUnavailable
	case ConnectionClosed:
		return ErrClosed
	}
	if conn == nil {
		return ErrUnavailable
	}
	if err := writeAll(conn, []byte(command+"\r\n")); err != nil {
		return errors.New("could not send DragonRealms command")
	}
	s.mu.Lock()
	if s.conn == conn && (s.state == ConnectionConnected || s.state == ConnectionReady) {
		normalized := strings.ToLower(strings.TrimSpace(command))
		s.reconnectArmed = normalized != "quit" && normalized != "exit"
	}
	s.mu.Unlock()
	return nil
}

// Updates returns ordered Session publications. The channel closes at termination.
func (s *Session) Updates() <-chan Update {
	return s.updates
}

// Close stops the session and joins every session goroutine.
func (s *Session) Close() error {
	s.closeOnce.Do(func() {
		conn, _ := s.transitionClosed(true)
		s.cancel()
		if conn != nil {
			_ = conn.Close()
		}
	})
	<-s.done
	return nil
}

func (s *Session) stateValue() ConnectionState {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.state
}

func (s *Session) watchCancellation() {
	defer s.wg.Done()
	<-s.ctx.Done()
	conn, _ := s.transitionClosed(false)
	if conn != nil {
		_ = conn.Close()
	}
}

func (s *Session) readLoop() {
	defer s.wg.Done()
	decoder := newStreamDecoder()
	reducer := newReducer(s.credentials.Character)
	buffer := make([]byte, 16*1024)

	for {
		conn := s.currentConn()
		if conn == nil {
			if s.ctx.Err() != nil {
				return
			}
			s.terminal(reducer, errors.New("DragonRealms connection unavailable"))
			return
		}
		if s.options.watchdog > 0 {
			_ = conn.SetReadDeadline(time.Now().Add(s.options.watchdog))
		}
		n, err := conn.Read(buffer)
		if n > 0 {
			if processErr := s.processActions(reducer, decoder.feed(buffer[:n])); processErr != nil {
				if s.ctx.Err() != nil || errors.Is(processErr, context.Canceled) {
					return
				}
				s.terminal(reducer, processErr)
				return
			}
		}
		if err == nil {
			continue
		}
		if s.ctx.Err() != nil || errors.Is(err, context.Canceled) {
			return
		}
		if processErr := s.processActions(reducer, decoder.finish()); processErr != nil {
			if s.ctx.Err() != nil || errors.Is(processErr, context.Canceled) {
				return
			}
			s.terminal(reducer, processErr)
			return
		}
		if s.reconnectEligible(err) {
			reconnectErr := s.reconnect(reducer)
			if reconnectErr == nil {
				decoder = newStreamDecoder()
				continue
			}
			if s.ctx.Err() != nil || errors.Is(reconnectErr, context.Canceled) {
				return
			}
			s.terminal(reducer, reconnectErr)
			return
		}
		s.terminal(reducer, err)
		return
	}
}

func (s *Session) processActions(reducer *reducer, actions []protocolAction) error {
	for _, action := range actions {
		wasReady := reducer.public.Connection == ConnectionReady
		update, publish := reducer.apply(action)
		if !wasReady && reducer.public.Connection == ConnectionReady {
			if err := s.sendSystem("look"); err != nil {
				return err
			}
			if err := s.sendSystem("flags"); err != nil {
				return err
			}
			if !s.markReady() {
				return context.Canceled
			}
		}
		if !publish {
			continue
		}
		if !s.publish(update) {
			return context.Canceled
		}
	}
	return nil
}

func (s *Session) sendSystem(command string) error {
	s.writeMu.Lock()
	defer s.writeMu.Unlock()
	s.mu.RLock()
	conn := s.conn
	state := s.state
	s.mu.RUnlock()
	if conn == nil || state == ConnectionClosed || state == ConnectionReconnecting {
		return ErrUnavailable
	}
	if err := writeAll(conn, []byte(command+"\r\n")); err != nil {
		return errors.New("could not send DragonRealms setup command")
	}
	return nil
}

func (s *Session) reconnectEligible(err error) bool {
	if errors.Is(err, io.EOF) || errors.Is(err, net.ErrClosed) {
		return false
	}
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.ctx.Err() == nil && s.state != ConnectionClosed && s.reconnectArmed && !s.callerClosing
}

func (s *Session) reconnect(reducer *reducer) error {
	if !s.beginReconnect() {
		return context.Canceled
	}
	reducer.public.Connection = ConnectionReconnecting
	if !s.publish(Update{Snapshot: reducer.snapshot()}) {
		return context.Canceled
	}

	var lastErr error
	for _, delay := range s.options.reconnectDelays {
		if err := s.options.wait(s.ctx, delay); err != nil {
			return err
		}
		attemptCtx, cancel := context.WithTimeout(s.ctx, s.options.attemptTimeout)
		conn, err := connectGame(attemptCtx, s.credentials, s.options)
		cancel()
		if err != nil {
			lastErr = err
			continue
		}
		if !s.replaceConn(conn) {
			return context.Canceled
		}
		reducer.resetTransient(s.credentials.Character)
		if !s.publish(Update{Snapshot: reducer.snapshot()}) {
			return context.Canceled
		}
		return nil
	}
	if lastErr == nil {
		lastErr = errors.New("DragonRealms reconnect attempts exhausted")
	}
	return lastErr
}

func (s *Session) terminal(reducer *reducer, source error) {
	if _, changed := s.transitionClosed(false); !changed {
		return
	}
	reducer.public.Connection = ConnectionClosed
	snapshot := reducer.snapshot()
	_ = s.publish(Update{Snapshot: snapshot, Err: sanitizedTerminalError(source)})
	s.cancel()
}

func (s *Session) publish(update Update) bool {
	sanitizeUpdate(&update)
	update.Snapshot = cloneSnapshot(update.Snapshot)
	update.Display = cloneDisplay(update.Display)
	select {
	case s.updates <- update:
		return true
	case <-s.ctx.Done():
		return false
	}
}

func (s *Session) currentConn() net.Conn {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.conn
}

func (s *Session) transitionClosed(caller bool) (net.Conn, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if caller {
		s.callerClosing = true
	}
	changed := s.state != ConnectionClosed
	s.state = ConnectionClosed
	return s.conn, changed
}

func (s *Session) beginReconnect() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.callerClosing || s.state == ConnectionClosed || s.ctx.Err() != nil {
		return false
	}
	s.state = ConnectionReconnecting
	return true
}

func (s *Session) markReady() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.callerClosing || s.state != ConnectionConnected || s.ctx.Err() != nil {
		return false
	}
	s.state = ConnectionReady
	return true
}

func (s *Session) replaceConn(conn net.Conn) bool {
	s.mu.Lock()
	if s.callerClosing || s.state == ConnectionClosed || s.ctx.Err() != nil {
		s.mu.Unlock()
		_ = conn.Close()
		return false
	}
	old := s.conn
	s.conn = conn
	s.state = ConnectionConnected
	s.reconnectArmed = false
	s.mu.Unlock()
	if old != nil {
		_ = old.Close()
	}
	return true
}

func sanitizedConnectError(err error) error {
	var authErr *sgeAuthError
	if errors.As(err, &authErr) {
		return authErr
	}
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return errors.New("could not connect to DragonRealms")
}

func sanitizedTerminalError(err error) error {
	if errors.Is(err, io.EOF) {
		return errors.New("DragonRealms connection closed")
	}
	if errors.Is(err, context.Canceled) {
		return context.Canceled
	}
	return errors.New("DragonRealms connection failed")
}

func waitContext(ctx context.Context, delay time.Duration) error {
	timer := time.NewTimer(delay)
	defer timer.Stop()
	select {
	case <-timer.C:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
