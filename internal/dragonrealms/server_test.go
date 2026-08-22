package dragonrealms

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"strconv"
	"sync"
	"testing"
	"time"
)

type protocolServers struct {
	t       *testing.T
	sge     net.Listener
	game    net.Listener
	errors  chan error
	wg      sync.WaitGroup
	close   sync.Once
	wait    sync.Once
	waitErr error
}

func startProtocolServers(t *testing.T) *protocolServers {
	t.Helper()
	game, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	sge, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		game.Close()
		t.Fatal(err)
	}
	servers := &protocolServers{t: t, sge: sge, game: game, errors: make(chan error, 2)}
	servers.wg.Add(2)
	go func() {
		defer servers.wg.Done()
		servers.errors <- servers.serveSGE()
	}()
	go func() {
		defer servers.wg.Done()
		servers.errors <- servers.serveGame()
	}()
	return servers
}

func (s *protocolServers) SessionOptions() sessionOptions {
	options := defaultSessionOptions()
	options.initialAttempts = 1
	options.wait = func(context.Context, time.Duration) error { return nil }
	options.authenticate = func(ctx context.Context, credentials Credentials) (sgeEntry, error) {
		dialer := &net.Dialer{}
		conn, err := dialer.DialContext(ctx, "tcp", s.sge.Addr().String())
		if err != nil {
			return sgeEntry{}, err
		}
		defer conn.Close()
		return runSGE(ctx, conn, credentials, false, sgeTLSResponseIdle)
	}
	options.dialGame = (&net.Dialer{}).DialContext
	return options
}

func (s *protocolServers) serveSGE() error {
	conn, err := s.sge.Accept()
	if err != nil {
		return err
	}
	defer conn.Close()
	reader := bufio.NewReader(conn)
	if err := expectLine(reader, "K\n"); err != nil {
		return err
	}
	key := bytes.Repeat([]byte{'K'}, 32)
	if _, err := conn.Write(key[:13]); err != nil {
		return err
	}
	if _, err := conn.Write(append(key[13:], '\n')); err != nil {
		return err
	}
	auth, err := reader.ReadBytes('\n')
	if err != nil {
		return err
	}
	password, _ := transformPassword("pass", key)
	wantAuth := append([]byte("A\tTEST\t"), password...)
	wantAuth = append(wantAuth, '\n')
	if !bytes.Equal(auth, wantAuth) {
		return errors.New("SGE authentication transcript differed")
	}
	if _, err := conn.Write([]byte("A\tOK\tVALID\n")); err != nil {
		return err
	}
	if err := expectLine(reader, "M\n"); err != nil {
		return err
	}
	if _, err := conn.Write([]byte("M\tDR\n")); err != nil {
		return err
	}
	if err := expectLine(reader, "G\tDR\n"); err != nil {
		return err
	}
	if _, err := conn.Write([]byte("G\tNORMAL\n")); err != nil {
		return err
	}
	if err := expectLine(reader, "C\n"); err != nil {
		return err
	}
	if _, err := conn.Write([]byte("C\ta\tb\tc\td\t101\tHero\n")); err != nil {
		return err
	}
	if err := expectLine(reader, "L\t101\tSTORM\n"); err != nil {
		return err
	}
	host, portText, err := net.SplitHostPort(s.game.Addr().String())
	if err != nil {
		return err
	}
	response := fmt.Sprintf("L\tKEY=local-game-key\tGAMEHOST=%s\tGAMEPORT=%s\n", host, portText)
	_, err = conn.Write([]byte(response))
	return err
}

func (s *protocolServers) serveGame() error {
	conn, err := s.game.Accept()
	if err != nil {
		return err
	}
	defer conn.Close()
	reader := bufio.NewReader(conn)
	if err := expectLine(reader, "local-game-key\n"); err != nil {
		return err
	}
	if err := expectLine(reader, "FE:GENIE /VERSION:5.0.0.1 /P:WIN_UNKNOWN /XML\n"); err != nil {
		return err
	}
	payload := []byte("<settingsInfo/><streamWindow id='room' subtitle=' - [Jalapeño Plaza] (77)'/><component id='room desc'>A sunny plaza.</component><prompt time='1'>&gt;</prompt>")
	marker := bytes.Index(payload, []byte("ñ"))
	if marker < 0 {
		return errors.New("integration payload lacks UTF-8 split marker")
	}
	if _, err := conn.Write(payload[:marker+1]); err != nil {
		return err
	}
	if _, err := conn.Write(payload[marker+1:]); err != nil {
		return err
	}
	for _, line := range []string{"look\r\n", "flags\r\n", "north\r\n"} {
		if err := expectLine(reader, line); err != nil {
			return err
		}
	}
	return nil
}

func (s *protocolServers) Wait() error {
	s.wait.Do(func() {
		s.wg.Wait()
		for i := 0; i < 2; i++ {
			if err := <-s.errors; err != nil && !errors.Is(err, net.ErrClosed) && s.waitErr == nil {
				s.waitErr = err
			}
		}
	})
	return s.waitErr
}

func (s *protocolServers) Close() {
	s.close.Do(func() {
		_ = s.sge.Close()
		_ = s.game.Close()
	})
	_ = s.Wait()
}

func expectLine(reader *bufio.Reader, want string) error {
	got, err := reader.ReadString('\n')
	if err != nil {
		return err
	}
	if got != want {
		return fmt.Errorf("protocol line differed: got %q, want %q", got, want)
	}
	return nil
}

func listenerPort(listener net.Listener) int {
	_, port, _ := net.SplitHostPort(listener.Addr().String())
	value, _ := strconv.Atoi(port)
	return value
}
