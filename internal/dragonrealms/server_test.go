package dragonrealms

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"net"
	"sync"
	"testing"
	"time"
)

type protocolServers struct {
	sge    net.Listener
	game   net.Listener
	done   chan struct{}
	result error
	close  sync.Once
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
	servers := &protocolServers{sge: sge, game: game, done: make(chan struct{})}
	go func() {
		defer close(servers.done)
		servers.result = servers.serveSGE()
		if servers.result == nil {
			servers.result = servers.serveGame()
		}
		if errors.Is(servers.result, net.ErrClosed) {
			servers.result = nil
		}
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
	<-s.done
	return s.result
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
