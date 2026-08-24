package dragonrealms

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"io"
	"net"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestDialRejectsInvalidCredentialsBeforeNetwork(t *testing.T) {
	t.Parallel()

	session, err := Dial(context.Background(), Credentials{})
	if err == nil || session != nil {
		t.Fatalf("Dial with invalid credentials = session %#v, err %v", session, err)
	}
	var authErr *sgeAuthError
	if !errors.As(err, &authErr) {
		t.Fatalf("Dial error = %T %v, want authentication error", err, err)
	}
}

func TestSessionHandshakeReadinessAndSend(t *testing.T) {
	conn := newScriptedConn()
	session, err := dialWithOptions(context.Background(), Credentials{Account: "a", Password: "p", Character: "Hero"}, testSessionOptions(conn))
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	if got := conn.waitForWrites(t, 2); got[0] != "game-key\n" || got[1] != "FE:GENIE /VERSION:5.0.0.1 /P:WIN_UNKNOWN /XML\n" {
		t.Fatalf("handshake writes = %#v", got)
	}
	if session.stateValue() != ConnectionConnected {
		t.Fatalf("state = %v", session.stateValue())
	}

	conn.reads <- readResult{data: []byte("<settingsInfo/><settingsInfo/><component id='room title'>[Square]</component><prompt time='1'>&gt;</prompt>")}
	lastConnection := ConnectionConnected
	readyTransitions := 0
	var ready Update
	for ready.Snapshot.Prompt != ">" {
		select {
		case ready = <-session.Updates():
			if lastConnection != ConnectionReady && ready.Snapshot.Connection == ConnectionReady {
				readyTransitions++
			}
			lastConnection = ready.Snapshot.Connection
		case <-time.After(time.Second):
			t.Fatal("ready update timed out")
		}
	}
	writes := conn.waitForWrites(t, 4)
	if readyTransitions != 1 || len(writes) != 4 || writes[2] != "look\r\n" || writes[3] != "flags\r\n" {
		t.Fatalf("ready transitions = %d, automatic writes = %#v", readyTransitions, writes)
	}

	if err := session.Send("say jalapeño"); err != nil {
		t.Fatal(err)
	}
	writes = conn.waitForWrites(t, 5)
	if writes[4] != "say jalapeño\r\n" || bytes.Contains([]byte(writes[4]), []byte{0xef, 0xbb, 0xbf}) {
		t.Fatalf("command write = %q", writes[4])
	}
}

func TestSendValidationAndSerialization(t *testing.T) {
	conn := newScriptedConn()
	session, err := dialWithOptions(context.Background(), Credentials{Account: "a", Password: "p", Character: "Hero"}, testSessionOptions(conn))
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	for _, command := range []string{"bad\ncommand", "bad\rcommand", "bad\x00command", string([]byte{0xff})} {
		if !errors.Is(session.Send(command), ErrInvalidCommand) {
			t.Fatalf("Send(%q) did not return ErrInvalidCommand", command)
		}
	}

	const commands = 32
	var wg sync.WaitGroup
	for i := 0; i < commands; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if err := session.Send("look"); err != nil {
				t.Errorf("Send: %v", err)
			}
		}()
	}
	wg.Wait()
	writes := conn.waitForWrites(t, commands+2)
	for _, write := range writes[2:] {
		if write != "look\r\n" {
			t.Fatalf("interleaved write = %q", write)
		}
	}
}

func TestSessionReconnectPolicy(t *testing.T) {
	first := newScriptedConn()
	second := newScriptedConn()
	options := testSessionOptions(first)
	var dialMu sync.Mutex
	dials := 0
	options.dialGame = func(context.Context, string, string) (net.Conn, error) {
		dialMu.Lock()
		defer dialMu.Unlock()
		dials++
		if dials == 1 {
			return first, nil
		}
		return second, nil
	}
	var delays []time.Duration
	options.wait = func(_ context.Context, delay time.Duration) error {
		delays = append(delays, delay)
		return nil
	}

	session, err := dialWithOptions(context.Background(), Credentials{Account: "a", Password: "p", Character: "Hero"}, options)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if err := session.Send("look"); err != nil {
		t.Fatal(err)
	}
	first.reads <- readResult{err: errors.New("connection reset")}

	var sawReconnecting bool
	deadline := time.After(time.Second)
	for !sawReconnecting {
		select {
		case update := <-session.Updates():
			sawReconnecting = update.Snapshot.Connection == ConnectionReconnecting
		case <-deadline:
			t.Fatal("reconnecting update timed out")
		}
	}
	second.waitForWrites(t, 2)
	if len(delays) != 1 || delays[0] != 8*time.Second {
		t.Fatalf("reconnect delays = %#v", delays)
	}
	reconnected := false
	for !reconnected {
		select {
		case update := <-session.Updates():
			reconnected = update.Snapshot.Connection == ConnectionConnected
		case <-deadline:
			t.Fatal("connected update timed out")
		}
	}
	if err := session.Send("look"); err != nil {
		t.Fatalf("Send after reconnect: %v", err)
	}
}

func TestReconnectRequiresFreshCommandOnNewConnection(t *testing.T) {
	for _, tt := range []struct {
		name      string
		freshSend bool
		wantDials int
	}{
		{name: "second drop before fresh command", wantDials: 2},
		{name: "second drop after fresh command", freshSend: true, wantDials: 3},
	} {
		t.Run(tt.name, func(t *testing.T) {
			connections := []*scriptedConn{newScriptedConn(), newScriptedConn(), newScriptedConn()}
			options := testSessionOptions(connections[0])
			var dialMu sync.Mutex
			dials := 0
			options.dialGame = func(context.Context, string, string) (net.Conn, error) {
				dialMu.Lock()
				defer dialMu.Unlock()
				if dials >= len(connections) {
					return nil, errors.New("unexpected extra game dial")
				}
				conn := connections[dials]
				dials++
				return conn, nil
			}
			session, err := dialWithOptions(context.Background(), Credentials{Account: "a", Password: "p", Character: "Hero"}, options)
			if err != nil {
				t.Fatal(err)
			}
			defer session.Close()

			waitForState := func(want ConnectionState) {
				t.Helper()
				for {
					select {
					case update, ok := <-session.Updates():
						if !ok {
							t.Fatalf("updates closed before state %v", want)
						}
						if update.Snapshot.Connection == want {
							return
						}
					case <-time.After(time.Second):
						t.Fatalf("state %v timed out", want)
					}
				}
			}

			if err := session.Send("look"); err != nil {
				t.Fatal(err)
			}
			connections[0].reads <- readResult{err: errors.New("first reset")}
			waitForState(ConnectionReconnecting)
			waitForState(ConnectionConnected)
			connections[1].waitForWrites(t, 2)

			if tt.freshSend {
				if err := session.Send("north"); err != nil {
					t.Fatal(err)
				}
				connections[1].waitForWrites(t, 3)
			}
			connections[1].reads <- readResult{err: errors.New("second reset")}

			if tt.freshSend {
				waitForState(ConnectionReconnecting)
				waitForState(ConnectionConnected)
				connections[2].waitForWrites(t, 2)
			} else {
				for {
					select {
					case update, ok := <-session.Updates():
						if !ok {
							goto closed
						}
						if update.Snapshot.Connection == ConnectionReconnecting {
							t.Fatal("second connection reconnected without fresh player input")
						}
					case <-time.After(time.Second):
						t.Fatal("second connection did not terminate")
					}
				}
			}

		closed:
			dialMu.Lock()
			gotDials := dials
			dialMu.Unlock()
			if gotDials != tt.wantDials {
				t.Fatalf("game dials = %d, want %d", gotDials, tt.wantDials)
			}
		})
	}
}

func TestLateSuccessfulSendDoesNotArmReplacement(t *testing.T) {
	for _, tt := range []struct {
		name      string
		freshSend bool
		wantDials int
	}{
		{name: "no fresh command", wantDials: 2},
		{name: "fresh command", freshSend: true, wantDials: 3},
	} {
		t.Run(tt.name, func(t *testing.T) {
			first := newLateSuccessfulCommandConn()
			connections := []net.Conn{first, newScriptedConn(), newScriptedConn()}
			options := testSessionOptions(first)
			var dialMu sync.Mutex
			dials := 0
			options.dialGame = func(context.Context, string, string) (net.Conn, error) {
				dialMu.Lock()
				defer dialMu.Unlock()
				if dials >= len(connections) {
					return nil, errors.New("unexpected extra game dial")
				}
				conn := connections[dials]
				dials++
				return conn, nil
			}

			session, err := dialWithOptions(context.Background(), Credentials{Account: "a", Password: "p", Character: "Hero"}, options)
			if err != nil {
				t.Fatal(err)
			}
			defer session.Close()
			defer first.releaseWrite()

			waitForState := func(want ConnectionState) {
				t.Helper()
				for {
					select {
					case update, ok := <-session.Updates():
						if !ok {
							t.Fatalf("updates closed before state %v", want)
						}
						if update.Snapshot.Connection == want {
							return
						}
					case <-time.After(time.Second):
						t.Fatalf("state %v timed out", want)
					}
				}
			}

			if err := session.Send("look"); err != nil {
				t.Fatal(err)
			}
			first.armNextCommand()
			sendDone := make(chan error, 1)
			go func() {
				sendDone <- session.Send("wait")
			}()
			select {
			case <-first.succeeded:
			case <-time.After(time.Second):
				t.Fatal("old command write did not succeed")
			}

			first.reads <- readResult{err: errors.New("first reset")}
			waitForState(ConnectionReconnecting)
			waitForState(ConnectionConnected)
			second := connections[1].(*scriptedConn)
			second.waitForWrites(t, 2)

			first.releaseWrite()
			select {
			case err := <-sendDone:
				if err != nil {
					t.Fatalf("late Send = %v", err)
				}
			case <-time.After(time.Second):
				t.Fatal("late Send did not return")
			}

			if tt.freshSend {
				if err := session.Send("north"); err != nil {
					t.Fatal(err)
				}
				second.waitForWrites(t, 3)
			}
			second.reads <- readResult{err: errors.New("second reset")}

			if tt.freshSend {
				waitForState(ConnectionReconnecting)
				waitForState(ConnectionConnected)
				connections[2].(*scriptedConn).waitForWrites(t, 2)
			} else {
				for {
					select {
					case update, ok := <-session.Updates():
						if !ok {
							goto closed
						}
						if update.Snapshot.Connection == ConnectionReconnecting {
							t.Fatal("late old-connection write armed replacement")
						}
					case <-time.After(time.Second):
						t.Fatal("second connection did not terminate")
					}
				}
			}

		closed:
			dialMu.Lock()
			gotDials := dials
			dialMu.Unlock()
			if gotDials != tt.wantDials {
				t.Fatalf("game dials = %d, want %d", gotDials, tt.wantDials)
			}
		})
	}
}

func TestReconnectArmingCommandPolicy(t *testing.T) {
	conn := newControlledCommandConn()
	session, err := dialWithOptions(context.Background(), Credentials{Account: "a", Password: "p", Character: "Hero"}, testSessionOptions(conn))
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	for _, tt := range []struct {
		name         string
		command      string
		writeFails   bool
		wantEligible bool
	}{
		{name: "empty arms", command: "", wantEligible: true},
		{name: "whitespace arms", command: "   ", wantEligible: true},
		{name: "quit disarms", command: " QuIt "},
		{name: "ordinary rearms after quit", command: "north", wantEligible: true},
		{name: "failed quit leaves armed", command: "quit", writeFails: true, wantEligible: true},
		{name: "exit disarms", command: " EXIT "},
		{name: "failed ordinary leaves disarmed", command: "look", writeFails: true},
		{name: "ordinary rearms after exit", command: "south", wantEligible: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			conn.setCommandFailure(tt.writeFails)
			err := session.Send(tt.command)
			if tt.writeFails && err == nil {
				t.Fatal("command write unexpectedly succeeded")
			}
			if !tt.writeFails && err != nil {
				t.Fatal(err)
			}
			if got := session.reconnectEligible(errors.New("reset")); got != tt.wantEligible {
				t.Fatalf("reconnect eligibility = %v, want %v", got, tt.wantEligible)
			}
		})
	}
}

func TestSessionDoesNotReconnectWithoutEligibleCommand(t *testing.T) {
	for _, tt := range []struct {
		name    string
		command string
		err     error
	}{
		{name: "no command", err: errors.New("reset")},
		{name: "quit", command: "quit", err: errors.New("reset")},
		{name: "exit", command: "exit", err: errors.New("reset")},
		{name: "clean EOF", command: "look", err: io.EOF},
	} {
		t.Run(tt.name, func(t *testing.T) {
			conn := newScriptedConn()
			options := testSessionOptions(conn)
			dials := 0
			options.dialGame = func(context.Context, string, string) (net.Conn, error) {
				dials++
				return conn, nil
			}
			session, err := dialWithOptions(context.Background(), Credentials{Account: "a", Password: "p", Character: "Hero"}, options)
			if err != nil {
				t.Fatal(err)
			}
			if tt.command != "" {
				if err := session.Send(tt.command); err != nil {
					t.Fatal(err)
				}
			}
			conn.reads <- readResult{err: tt.err}
			for range session.Updates() {
			}
			if dials != 1 {
				t.Fatalf("game dial count = %d", dials)
			}
			if err := session.Close(); err != nil {
				t.Fatal(err)
			}
		})
	}
}

func TestReconnectExhaustsExactDelayLadder(t *testing.T) {
	conn := newScriptedConn()
	options := testSessionOptions(conn)
	dials := 0
	options.dialGame = func(context.Context, string, string) (net.Conn, error) {
		dials++
		if dials == 1 {
			return conn, nil
		}
		return nil, errors.New("game endpoint unavailable")
	}
	var delays []time.Duration
	options.wait = func(_ context.Context, delay time.Duration) error {
		delays = append(delays, delay)
		return nil
	}
	session, err := dialWithOptions(context.Background(), Credentials{Account: "a", Password: "p", Character: "Hero"}, options)
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Send("look"); err != nil {
		t.Fatal(err)
	}
	conn.reads <- readResult{err: errors.New("reset")}
	for range session.Updates() {
	}
	want := []time.Duration{8 * time.Second, 8 * time.Second, 30 * time.Second, 30 * time.Second, 30 * time.Second}
	if dials != 6 || !reflect.DeepEqual(delays, want) {
		t.Fatalf("dials = %d, delays = %#v", dials, delays)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestReconnectExhaustionPublishesReconnectFailure(t *testing.T) {
	conn := newScriptedConn()
	options := testSessionOptions(conn)
	dials := 0
	options.dialGame = func(context.Context, string, string) (net.Conn, error) {
		dials++
		if dials == 1 {
			return conn, nil
		}
		return nil, io.EOF
	}
	session, err := dialWithOptions(context.Background(), Credentials{Account: "a", Password: "p", Character: "Hero"}, options)
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Send("look"); err != nil {
		t.Fatal(err)
	}
	conn.reads <- readResult{err: errors.New("generic read reset")}

	var terminal error
	for update := range session.Updates() {
		if update.Err != nil {
			terminal = update.Err
		}
	}
	if terminal == nil || terminal.Error() != "DragonRealms connection closed" {
		t.Fatalf("terminal error = %v, want reconnect EOF classification", terminal)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestCallerCancellationDoesNotReconnect(t *testing.T) {
	conn := newScriptedConn()
	options := testSessionOptions(conn)
	dials := 0
	options.dialGame = func(context.Context, string, string) (net.Conn, error) {
		dials++
		return conn, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	session, err := dialWithOptions(ctx, Credentials{Account: "a", Password: "p", Character: "Hero"}, options)
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Send("look"); err != nil {
		t.Fatal(err)
	}
	cancel()
	for range session.Updates() {
	}
	if dials != 1 {
		t.Fatalf("game dial count after cancellation = %d", dials)
	}
	if session.stateValue() != ConnectionClosed {
		t.Fatalf("state after cancellation = %v, want closed", session.stateValue())
	}
	if err := session.Send("look"); !errors.Is(err, ErrClosed) {
		t.Fatalf("Send after cancellation = %v, want ErrClosed", err)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestCloseBetweenReconnectEligibilityAndTransitionStaysClosed(t *testing.T) {
	conn := newScriptedConn()
	session, err := dialWithOptions(context.Background(), Credentials{Account: "a", Password: "p", Character: "Hero"}, testSessionOptions(conn))
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Send("look"); err != nil {
		t.Fatal(err)
	}
	if !session.reconnectEligible(errors.New("reset")) {
		t.Fatal("expected reconnect eligibility")
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	if err := session.reconnect(newReducer("Hero")); !errors.Is(err, context.Canceled) {
		t.Fatalf("reconnect after Close = %v, want context cancellation", err)
	}
	if session.stateValue() != ConnectionClosed {
		t.Fatalf("state after rejected reconnect = %v, want closed", session.stateValue())
	}
	if err := session.Send("north"); !errors.Is(err, ErrClosed) {
		t.Fatalf("Send after rejected reconnect = %v, want ErrClosed", err)
	}
}

func TestCommandWriteFailureDoesNotArmReconnect(t *testing.T) {
	conn := &commandWriteErrorConn{scriptedConn: newScriptedConn()}
	options := testSessionOptions(conn)
	dials := 0
	options.dialGame = func(context.Context, string, string) (net.Conn, error) {
		dials++
		return conn, nil
	}
	session, err := dialWithOptions(context.Background(), Credentials{Account: "a", Password: "p", Character: "Hero"}, options)
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Send("look"); err == nil {
		t.Fatal("command write failure was not returned")
	}
	conn.reads <- readResult{err: errors.New("reset")}
	for range session.Updates() {
	}
	if dials != 1 {
		t.Fatalf("game dial count after command write failure = %d", dials)
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
}

func TestSendReturnsUnavailableDuringReconnect(t *testing.T) {
	first := newScriptedConn()
	second := newScriptedConn()
	options := testSessionOptions(first)
	dials := 0
	options.dialGame = func(context.Context, string, string) (net.Conn, error) {
		dials++
		if dials == 1 {
			return first, nil
		}
		return second, nil
	}
	release := make(chan struct{})
	options.wait = func(ctx context.Context, _ time.Duration) error {
		select {
		case <-release:
			return nil
		case <-ctx.Done():
			return ctx.Err()
		}
	}
	session, err := dialWithOptions(context.Background(), Credentials{Account: "a", Password: "p", Character: "Hero"}, options)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if err := session.Send("look"); err != nil {
		t.Fatal(err)
	}
	first.reads <- readResult{err: errors.New("reset")}
	for {
		update := <-session.Updates()
		if update.Snapshot.Connection == ConnectionReconnecting {
			break
		}
	}
	if err := session.Send("north"); !errors.Is(err, ErrUnavailable) {
		t.Fatalf("Send during reconnect = %v", err)
	}
	close(release)
	second.waitForWrites(t, 2)
}

func TestCloseCancelsBlockedReconnectWait(t *testing.T) {
	conn := newScriptedConn()
	options := testSessionOptions(conn)
	dials := 0
	options.dialGame = func(context.Context, string, string) (net.Conn, error) {
		dials++
		return conn, nil
	}
	entered := make(chan struct{}, 1)
	options.wait = func(ctx context.Context, _ time.Duration) error {
		entered <- struct{}{}
		<-ctx.Done()
		return ctx.Err()
	}
	session, err := dialWithOptions(context.Background(), Credentials{Account: "a", Password: "p", Character: "Hero"}, options)
	if err != nil {
		t.Fatal(err)
	}
	if err := session.Send("look"); err != nil {
		t.Fatal(err)
	}
	conn.reads <- readResult{err: errors.New("generic read reset")}
	select {
	case <-entered:
	case <-time.After(time.Second):
		t.Fatal("reconnect wait was not entered")
	}

	closed := make(chan error, 1)
	go func() { closed <- session.Close() }()
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("Close did not cancel reconnect wait")
	}
	for update := range session.Updates() {
		if update.Err != nil {
			t.Fatalf("canceled reconnect published terminal error: %v", update.Err)
		}
	}
	if dials != 1 {
		t.Fatalf("game dials = %d, want 1", dials)
	}
}

func TestCloseCancelsBlockedReadyPublication(t *testing.T) {
	conn := newScriptedConn()
	options := testSessionOptions(conn)
	options.updateCapacity = 1
	session, err := dialWithOptions(context.Background(), Credentials{Account: "a", Password: "p", Character: "Hero"}, options)
	if err != nil {
		t.Fatal(err)
	}
	session.updates <- Update{}
	conn.reads <- readResult{data: []byte("<settingsInfo/>")}
	conn.waitForWrites(t, 4)

	readyDeadline := time.NewTimer(time.Second)
	defer readyDeadline.Stop()
	readyPoll := time.NewTicker(time.Millisecond)
	defer readyPoll.Stop()
	for session.stateValue() != ConnectionReady {
		select {
		case <-readyPoll.C:
		case <-readyDeadline.C:
			t.Fatal("session did not reach ready state")
		}
	}

	closed := make(chan error, 1)
	go func() { closed <- session.Close() }()
	select {
	case err := <-closed:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("Close did not cancel blocked ready publication")
	}
	for update := range session.Updates() {
		if update.Err != nil {
			t.Fatalf("canceled ready publication produced terminal error: %v", update.Err)
		}
	}
}

func TestWatchdogSetsApplicationDeadline(t *testing.T) {
	conn := newScriptedConn()
	options := testSessionOptions(conn)
	options.watchdog = 5 * time.Minute
	session, err := dialWithOptions(context.Background(), Credentials{Account: "a", Password: "p", Character: "Hero"}, options)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	select {
	case deadline := <-conn.deadlines:
		remaining := time.Until(deadline)
		if remaining < 4*time.Minute+59*time.Second || remaining > 5*time.Minute {
			t.Fatalf("watchdog deadline remaining = %v", remaining)
		}
	case <-time.After(time.Second):
		t.Fatal("watchdog deadline was not set")
	}
}

func TestSessionInitialRetries(t *testing.T) {
	conn := newScriptedConn()
	options := testSessionOptions(conn)
	options.initialAttempts = 3
	authCalls := 0
	options.authenticate = func(context.Context, Credentials) (sgeEntry, error) {
		authCalls++
		if authCalls < 3 {
			return sgeEntry{}, errors.New("temporary transport failure")
		}
		return sgeEntry{host: "game", port: 1, key: "game-key"}, nil
	}
	var waits []time.Duration
	options.wait = func(_ context.Context, delay time.Duration) error {
		waits = append(waits, delay)
		return nil
	}

	session, err := dialWithOptions(context.Background(), Credentials{Account: "a", Password: "p", Character: "Hero"}, options)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()
	if authCalls != 3 || len(waits) != 2 || waits[0] != 5*time.Second {
		t.Fatalf("auth calls = %d, waits = %#v", authCalls, waits)
	}
}

func TestSessionAttemptDeadlineStopsRetriesButTransportErrorsDoNot(t *testing.T) {
	t.Run("attempt deadline", func(t *testing.T) {
		options := defaultSessionOptions()
		options.initialAttempts = 3
		options.attemptTimeout = 10 * time.Millisecond
		calls := 0
		options.authenticate = func(ctx context.Context, _ Credentials) (sgeEntry, error) {
			calls++
			<-ctx.Done()
			return sgeEntry{}, ctx.Err()
		}
		options.wait = func(context.Context, time.Duration) error {
			t.Fatal("wait called after per-attempt deadline")
			return nil
		}
		_, err := dialWithOptions(context.Background(), Credentials{Account: "a", Password: "p", Character: "Hero"}, options)
		if !errors.Is(err, context.DeadlineExceeded) || calls != 1 {
			t.Fatalf("deadline result = %v, calls = %d", err, calls)
		}
	})

	t.Run("transport failure", func(t *testing.T) {
		options := defaultSessionOptions()
		options.initialAttempts = 3
		calls := 0
		waits := 0
		options.authenticate = func(context.Context, Credentials) (sgeEntry, error) {
			calls++
			return sgeEntry{}, errors.New("temporary transport failure")
		}
		options.wait = func(context.Context, time.Duration) error {
			waits++
			return nil
		}
		if _, err := dialWithOptions(context.Background(), Credentials{Account: "a", Password: "p", Character: "Hero"}, options); err == nil || calls != 3 || waits != 2 {
			t.Fatalf("transport result = %v, calls = %d, waits = %d", err, calls, waits)
		}
	})
}

func TestGameHandshakeCancellationClosesBlockedTransport(t *testing.T) {
	client, server := net.Pipe()
	defer server.Close()
	options := defaultSessionOptions()
	options.authenticate = func(context.Context, Credentials) (sgeEntry, error) {
		return sgeEntry{host: "game", port: 1, key: "game-key"}, nil
	}
	options.dialGame = func(context.Context, string, string) (net.Conn, error) {
		return client, nil
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := connectGame(ctx, Credentials{Account: "a", Password: "p", Character: "Hero"}, options)
		done <- err
	}()
	reader := bufio.NewReader(server)
	if line, err := reader.ReadString('\n'); err != nil || line != "game-key\n" {
		t.Fatalf("game key = %q, err %v", line, err)
	}
	cancel()
	select {
	case err := <-done:
		if err == nil {
			t.Fatal("canceled game handshake succeeded")
		}
	case <-time.After(time.Second):
		t.Fatal("canceled game handshake left its transport blocked")
	}
}

func TestSessionDoesNotRetryAuthenticationFailure(t *testing.T) {
	for _, tt := range []struct {
		name         string
		authenticate func(context.Context, Credentials) (sgeEntry, error)
		secret       string
	}{
		{
			name: "rejected password",
			authenticate: func(context.Context, Credentials) (sgeEntry, error) {
				return sgeEntry{}, &sgeAuthError{kind: "password"}
			},
		},
		{
			name:   "incomplete final login",
			secret: "sensitive-game-key",
			authenticate: func(context.Context, Credentials) (sgeEntry, error) {
				return parseLoginResponse("L\tKEY=sensitive-game-key\tGAMEPORT=4900")
			},
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			options := defaultSessionOptions()
			calls := 0
			options.authenticate = func(ctx context.Context, credentials Credentials) (sgeEntry, error) {
				calls++
				return tt.authenticate(ctx, credentials)
			}
			options.wait = func(context.Context, time.Duration) error {
				t.Fatal("wait called for authentication failure")
				return nil
			}
			_, err := dialWithOptions(context.Background(), Credentials{Account: "a", Password: "p", Character: "Hero"}, options)
			if err == nil || calls != 1 {
				t.Fatalf("err = %v, calls = %d", err, calls)
			}
			if tt.secret != "" && strings.Contains(err.Error(), tt.secret) {
				t.Fatalf("error leaked final-login response: %v", err)
			}
		})
	}
}

func TestCloseCancelsSaturatedDeliveryAndIsIdempotent(t *testing.T) {
	conn := newScriptedConn()
	session, err := dialWithOptions(context.Background(), Credentials{Account: "a", Password: "p", Character: "Hero"}, testSessionOptions(conn))
	if err != nil {
		t.Fatal(err)
	}
	conn.reads <- readResult{data: []byte(strings.Repeat("<clearStream id='main'/>", 300))}

	done := make(chan error, 1)
	go func() { done <- session.Close() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatal(err)
		}
	case <-time.After(time.Second):
		t.Fatal("Close blocked behind full Updates channel")
	}
	if err := session.Close(); err != nil {
		t.Fatal(err)
	}
	for range session.Updates() {
	}
	if !conn.isClosed() {
		t.Fatal("transport remained open")
	}
	if err := session.Send("look"); !errors.Is(err, ErrClosed) {
		t.Fatalf("Send after Close = %v, want ErrClosed", err)
	}
}

func TestSessionActiveDeliveryIsLosslessUnderBackpressure(t *testing.T) {
	conn := newScriptedConn()
	options := testSessionOptions(conn)
	options.updateCapacity = 4
	session, err := dialWithOptions(context.Background(), Credentials{Account: "a", Password: "p", Character: "Hero"}, options)
	if err != nil {
		t.Fatal(err)
	}
	defer session.Close()

	const updates = 40
	conn.reads <- readResult{data: []byte(strings.Repeat("<clearStream id='main'/>", updates))}
	for i := 0; i < updates; i++ {
		select {
		case update := <-session.Updates():
			if len(update.Display) != 1 || update.Display[0].Kind != DisplayClear {
				t.Fatalf("update %d = %#v", i, update)
			}
		case <-time.After(time.Second):
			t.Fatalf("received %d of %d active updates", i, updates)
		}
	}
}

func TestSessionFinishesDecoderBeforeTerminalClose(t *testing.T) {
	conn := newScriptedConn()
	session, err := dialWithOptions(context.Background(), Credentials{Account: "a", Password: "p", Character: "Hero"}, testSessionOptions(conn))
	if err != nil {
		t.Fatal(err)
	}
	conn.reads <- readResult{data: []byte("<component id='room title'"), err: io.EOF}

	var sawIncomplete, sawTerminal bool
	for update := range session.Updates() {
		for _, diagnostic := range update.Diagnostics {
			if strings.Contains(diagnostic.Text, "incomplete") {
				sawIncomplete = true
			}
		}
		if update.Err != nil && update.Snapshot.Connection == ConnectionClosed {
			sawTerminal = true
		}
	}
	if !sawIncomplete || !sawTerminal {
		t.Fatalf("EOF publications: incomplete=%v terminal=%v", sawIncomplete, sawTerminal)
	}
}

func TestSpontaneousFailurePublishesErrorBeforeClosing(t *testing.T) {
	conn := newScriptedConn()
	session, err := dialWithOptions(context.Background(), Credentials{Account: "a", Password: "p", Character: "Hero"}, testSessionOptions(conn))
	if err != nil {
		t.Fatal(err)
	}
	conn.reads <- readResult{err: errors.New("socket exploded with secret-token")}
	update, ok := <-session.Updates()
	if !ok || update.Err == nil || update.Snapshot.Connection != ConnectionClosed {
		t.Fatalf("terminal update = %#v, ok %v", update, ok)
	}
	if strings.Contains(update.Err.Error(), "secret-token") {
		t.Fatalf("terminal error leaked source detail: %v", update.Err)
	}
	if _, ok := <-session.Updates(); ok {
		t.Fatal("Updates remained open after terminal error")
	}
}

func testSessionOptions(conn net.Conn) sessionOptions {
	options := defaultSessionOptions()
	options.authenticate = func(context.Context, Credentials) (sgeEntry, error) {
		return sgeEntry{host: "game", port: 1, key: "game-key"}, nil
	}
	options.dialGame = func(context.Context, string, string) (net.Conn, error) { return conn, nil }
	options.wait = func(context.Context, time.Duration) error { return nil }
	options.initialAttempts = 1
	return options
}

type readResult struct {
	data []byte
	err  error
}

type scriptedConn struct {
	reads     chan readResult
	deadlines chan time.Time

	mu     sync.Mutex
	writes []string
	closed bool
	notify chan struct{}
}

type commandWriteErrorConn struct {
	*scriptedConn
}

func (c *commandWriteErrorConn) Write(data []byte) (int, error) {
	if bytes.HasSuffix(data, []byte("\r\n")) {
		return 0, errors.New("command write failed")
	}
	return c.scriptedConn.Write(data)
}

type lateSuccessfulCommandConn struct {
	*scriptedConn

	controlMu sync.Mutex
	blockNext bool
	succeeded chan struct{}
	release   chan struct{}
	released  sync.Once
}

func newLateSuccessfulCommandConn() *lateSuccessfulCommandConn {
	return &lateSuccessfulCommandConn{
		scriptedConn: newScriptedConn(),
		succeeded:    make(chan struct{}),
		release:      make(chan struct{}),
	}
}

func (c *lateSuccessfulCommandConn) armNextCommand() {
	c.controlMu.Lock()
	c.blockNext = true
	c.controlMu.Unlock()
}

func (c *lateSuccessfulCommandConn) releaseWrite() {
	c.released.Do(func() { close(c.release) })
}

func (c *lateSuccessfulCommandConn) Write(data []byte) (int, error) {
	if !bytes.HasSuffix(data, []byte("\r\n")) {
		return c.scriptedConn.Write(data)
	}
	c.controlMu.Lock()
	block := c.blockNext
	c.blockNext = false
	c.controlMu.Unlock()

	n, err := c.scriptedConn.Write(data)
	if !block || err != nil {
		return n, err
	}
	close(c.succeeded)
	<-c.release
	return n, err
}

func (c *lateSuccessfulCommandConn) Close() error {
	return c.scriptedConn.Close()
}

type controlledCommandConn struct {
	*scriptedConn

	controlMu sync.RWMutex
	fail      bool
}

func newControlledCommandConn() *controlledCommandConn {
	return &controlledCommandConn{scriptedConn: newScriptedConn()}
}

func (c *controlledCommandConn) setCommandFailure(fail bool) {
	c.controlMu.Lock()
	c.fail = fail
	c.controlMu.Unlock()
}

func (c *controlledCommandConn) Write(data []byte) (int, error) {
	c.controlMu.RLock()
	fail := c.fail
	c.controlMu.RUnlock()
	if fail && bytes.HasSuffix(data, []byte("\r\n")) {
		return 0, errors.New("command write failed")
	}
	return c.scriptedConn.Write(data)
}

func newScriptedConn() *scriptedConn {
	return &scriptedConn{reads: make(chan readResult, 512), deadlines: make(chan time.Time, 512), notify: make(chan struct{}, 512)}
}

func (c *scriptedConn) Read(p []byte) (int, error) {
	result, ok := <-c.reads
	if !ok {
		return 0, net.ErrClosed
	}
	n := copy(p, result.data)
	return n, result.err
}

func (c *scriptedConn) Write(p []byte) (int, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.closed {
		return 0, net.ErrClosed
	}
	c.writes = append(c.writes, string(append([]byte(nil), p...)))
	c.notify <- struct{}{}
	return len(p), nil
}

func (c *scriptedConn) Close() error {
	c.mu.Lock()
	if !c.closed {
		c.closed = true
		close(c.reads)
	}
	c.mu.Unlock()
	return nil
}

func (c *scriptedConn) LocalAddr() net.Addr         { return fakeAddr("local") }
func (c *scriptedConn) RemoteAddr() net.Addr        { return fakeAddr("remote") }
func (c *scriptedConn) SetDeadline(time.Time) error { return nil }
func (c *scriptedConn) SetReadDeadline(deadline time.Time) error {
	select {
	case c.deadlines <- deadline:
	default:
	}
	return nil
}
func (c *scriptedConn) SetWriteDeadline(time.Time) error { return nil }

func (c *scriptedConn) waitForWrites(t *testing.T, count int) []string {
	t.Helper()
	deadline := time.After(time.Second)
	for {
		c.mu.Lock()
		if len(c.writes) >= count {
			result := append([]string(nil), c.writes...)
			c.mu.Unlock()
			return result
		}
		c.mu.Unlock()
		select {
		case <-c.notify:
		case <-deadline:
			t.Fatalf("got fewer than %d writes", count)
		}
	}
}

func (c *scriptedConn) isClosed() bool {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.closed
}

type fakeAddr string

func (a fakeAddr) Network() string { return string(a) }
func (a fakeAddr) String() string  { return string(a) }
