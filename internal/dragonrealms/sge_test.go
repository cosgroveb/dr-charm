package dragonrealms

import (
	"bufio"
	"bytes"
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCredentialsRequireASCII(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		creds Credentials
		want  bool
	}{
		{name: "valid", creds: Credentials{Account: "account", Password: "password", Character: "Hero"}, want: true},
		{name: "empty account", creds: Credentials{Password: "password", Character: "Hero"}},
		{name: "empty password", creds: Credentials{Account: "account", Character: "Hero"}},
		{name: "empty character", creds: Credentials{Account: "account", Password: "password"}},
		{name: "control byte", creds: Credentials{Account: "account", Password: "pass\tword", Character: "Hero"}},
		{name: "delete byte", creds: Credentials{Account: "account", Password: "password", Character: "Hero\x7f"}},
		{name: "non ASCII account", creds: Credentials{Account: "accóunt", Password: "password", Character: "Hero"}},
		{name: "non ASCII password", creds: Credentials{Account: "account", Password: "pässword", Character: "Hero"}},
		{name: "non ASCII character", creds: Credentials{Account: "account", Password: "password", Character: "Héro"}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := validateCredentials(tt.creds)
			if (err == nil) != tt.want {
				t.Fatalf("validateCredentials() error = %v, want valid %v", err, tt.want)
			}
		})
	}
	if got := asciiUpper("mixed-z_é"); got != "MIXED-Z_é" {
		t.Fatalf("asciiUpper = %q", got)
	}
}

func TestPasswordTransformWrapsKey(t *testing.T) {
	t.Parallel()

	key := bytes.Repeat([]byte{'K'}, 32)
	password := strings.Repeat("A", 33)
	got, err := transformPassword(password, key)
	if err != nil {
		t.Fatal(err)
	}
	wantByte := byte((('A' - 32) ^ 'K') + 32)
	for _, i := range []int{0, 31, 32} {
		if got[i] != wantByte {
			t.Fatalf("byte %d = %d, want %d", i, got[i], wantByte)
		}
	}
}

func TestReadSGEKeyFraming(t *testing.T) {
	t.Parallel()

	key := bytes.Repeat([]byte{'x'}, 32)
	for _, tt := range []struct {
		name string
		tls  bool
		in   []byte
		rest string
		ok   bool
	}{
		{name: "TLS has no newline", tls: true, in: append(append([]byte{}, key...), []byte("next")...), rest: "next", ok: true},
		{name: "plaintext consumes newline", in: append(append([]byte{}, key...), []byte("\nnext")...), rest: "next", ok: true},
		{name: "plaintext requires newline", in: append(append([]byte{}, key...), 'x'), ok: false},
		{name: "short key", in: key[:31], ok: false},
	} {
		t.Run(tt.name, func(t *testing.T) {
			r := bufio.NewReader(bytes.NewReader(tt.in))
			got, err := readSGEKey(r, tt.tls)
			if (err == nil) != tt.ok {
				t.Fatalf("readSGEKey() error = %v, want success %v", err, tt.ok)
			}
			if !tt.ok {
				return
			}
			if !bytes.Equal(got, key) {
				t.Fatalf("key = %q, want %q", got, key)
			}
			rest, _ := io.ReadAll(r)
			if string(rest) != tt.rest {
				t.Fatalf("surplus = %q, want %q", rest, tt.rest)
			}
		})
	}
}

func TestReadSGEResponseFramingAndFragmentation(t *testing.T) {
	t.Parallel()

	plain := newScriptedConn()
	plainReader := bufio.NewReader(plain)
	plain.reads <- readResult{data: []byte("A\t")}
	plain.reads <- readResult{data: []byte("OK\nnext\n")}
	got, err := readSGEResponse(context.Background(), plain, plainReader, false, time.Second)
	if err != nil || got != "A\tOK" {
		t.Fatalf("plaintext response = %q, err %v", got, err)
	}
	next, err := plainReader.ReadString('\n')
	if err != nil || next != "next\n" {
		t.Fatalf("plaintext surplus = %q, err %v", next, err)
	}
	_ = plain.Close()

	tlsConn := newScriptedConn()
	tlsReader := bufio.NewReader(tlsConn)
	tlsConn.reads <- readResult{data: []byte("A\t")}
	tlsConn.reads <- readResult{data: []byte("OK")}
	tlsConn.reads <- readResult{err: timeoutError{}}
	got, err = readSGEResponse(context.Background(), tlsConn, tlsReader, true, time.Second)
	if err != nil || got != "A\tOK" {
		t.Fatalf("TLS response = %q, err %v", got, err)
	}
	_ = tlsConn.Close()
}

func TestTLSResponseIdleDeadlineStartsAfterFirstByte(t *testing.T) {
	t.Parallel()

	const idle = 200 * time.Millisecond
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	ctxDeadline, _ := ctx.Deadline()
	conn := &delayedTLSResponseConn{firstByteAt: time.Now().Add(2 * idle), response: []byte("A\tOK")}
	got, err := readSGEResponse(ctx, conn, bufio.NewReader(conn), true, idle)
	if err != nil || got != "A\tOK" {
		t.Fatalf("TLS response = %q, err %v", got, err)
	}
	if len(conn.deadlines) != 3 {
		t.Fatalf("read deadlines = %#v, want first-byte, idle, and clear", conn.deadlines)
	}
	if !conn.deadlines[0].deadline.Equal(ctxDeadline) {
		t.Fatalf("first-byte deadline = %v, want context deadline %v", conn.deadlines[0].deadline, ctxDeadline)
	}
	if conn.deadlines[1].deadline.IsZero() || !conn.deadlines[1].deadline.Before(ctxDeadline) {
		t.Fatalf("post-data deadline = %v, want idle deadline before %v", conn.deadlines[1].deadline, ctxDeadline)
	}
	if !conn.deadlines[2].deadline.IsZero() {
		t.Fatalf("final read deadline = %v, want cleared", conn.deadlines[2].deadline)
	}
}

func TestParseCharactersStartsAtFieldFive(t *testing.T) {
	t.Parallel()

	got, err := parseCharacters("C\ta\tb\tc\td\t101\tHero\t102\tOther")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0] != (sgeCharacter{code: "101", name: "Hero"}) || got[1] != (sgeCharacter{code: "102", name: "Other"}) {
		t.Fatalf("characters = %#v", got)
	}
	if _, err := parseCharacters("C\ta\tb\tc\td\t101"); err == nil {
		t.Fatal("malformed pair accepted")
	}
}

func TestParseLoginResponse(t *testing.T) {
	t.Parallel()

	got, err := parseLoginResponse("L\tOK\tGAMEPORT=4900\tKEY=secret\tGAMEHOST=game.example")
	if err != nil {
		t.Fatal(err)
	}
	if got.host != "game.example" || got.port != 4900 || got.key != "secret" {
		t.Fatalf("entry = %#v", got)
	}

	for _, response := range []string{
		"L\tKEY=secret\tGAMEHOST=game.example\tGAMEPORT=0",
		"L\tKEY=secret\tGAMEHOST=game.example\tGAMEPORT=not-a-port",
		"L\tKEY=secret\tGAMEHOST=game.example\tGAMEPORT=65536",
		"L\tGAMEHOST=game.example\tGAMEPORT=4900",
		"L\tKEY=secret\tGAMEPORT=4900",
	} {
		if _, err := parseLoginResponse(response); err == nil {
			t.Fatalf("parseLoginResponse(%q) succeeded", response)
		} else if strings.Contains(err.Error(), "secret") || strings.Contains(err.Error(), response) {
			t.Fatalf("error leaked response data: %v", err)
		}
	}
	for _, response := range []string{
		"L\tKEY=sensitive-game-key\tGAMEPORT=4900",
		"L\tGAMEHOST=game.example\tGAMEPORT=4900",
	} {
		_, err := parseLoginResponse(response)
		var authErr *sgeAuthError
		if !errors.As(err, &authErr) || authErr.kind != "malformed" {
			t.Fatalf("incomplete response error = %T %v, want malformed authentication error", err, err)
		}
		if strings.Contains(err.Error(), "sensitive") || strings.Contains(err.Error(), "GAME") || strings.Contains(err.Error(), response) {
			t.Fatalf("incomplete response error leaked response data: %v", err)
		}
	}
	for code := 1; code <= 4; code++ {
		response := fmt.Sprintf("L\t\tPrObLeM %d", code)
		_, err := parseLoginResponse(response)
		var authErr *sgeAuthError
		if !errors.As(err, &authErr) || authErr.kind != fmt.Sprintf("problem-%d", code) {
			t.Fatalf("parseLoginResponse(%q) = %T %v", response, err, err)
		}
	}
	if _, err := parseLoginResponse("L\tstatus\tPROBLEM UNKNOWN"); err == nil {
		t.Fatal("unknown PROBLEM response succeeded")
	}
}

func TestValidateAuthResponseFieldLayout(t *testing.T) {
	t.Parallel()

	for _, response := range []string{"A\t\tPASSWORD", "a\t\tunknown account", "invalid"} {
		if err := validateAuthResponse(response); err == nil {
			t.Fatalf("validateAuthResponse(%q) succeeded", response)
		}
	}
	if err := validateAuthResponse("A\tACCOUNT\tKEY\thash\tName"); err != nil {
		t.Fatalf("valid auth response failed: %v", err)
	}
}

func TestSGEProtocolTranscript(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	creds := Credentials{Account: "test_account", Password: "password", Character: "Hero"}
	serverErr := make(chan error, 1)
	go func() {
		reader := bufio.NewReader(server)
		wantLine := func(want string) error {
			got, err := reader.ReadString('\n')
			if err != nil {
				return err
			}
			if got != want {
				return errors.New("unexpected SGE command")
			}
			return nil
		}
		if err := wantLine("K\n"); err != nil {
			serverErr <- err
			return
		}
		key := bytes.Repeat([]byte{'K'}, 32)
		if _, err := server.Write(append(key, '\n')); err != nil {
			serverErr <- err
			return
		}
		auth, err := reader.ReadBytes('\n')
		if err != nil {
			serverErr <- err
			return
		}
		prefix := []byte("A\tTEST_ACCOUNT\t")
		enc, _ := transformPassword(creds.Password, key)
		wantAuth := append(append(append([]byte{}, prefix...), enc...), '\n')
		if !bytes.Equal(auth, wantAuth) {
			serverErr <- errors.New("unexpected auth bytes")
			return
		}
		if _, err := server.Write([]byte("A\tOK\tVALID\n")); err != nil {
			serverErr <- err
			return
		}
		if err := wantLine("M\n"); err != nil {
			serverErr <- err
			return
		}
		if _, err := server.Write([]byte("M\tDR\n")); err != nil {
			serverErr <- err
			return
		}
		if err := wantLine("G\tDR\n"); err != nil {
			serverErr <- err
			return
		}
		if _, err := server.Write([]byte("G\tNORMAL\tPREMIUM\n")); err != nil {
			serverErr <- err
			return
		}
		if err := wantLine("C\n"); err != nil {
			serverErr <- err
			return
		}
		if _, err := server.Write([]byte("C\ta\tb\tc\td\t101\tHero\t102\tOther\n")); err != nil {
			serverErr <- err
			return
		}
		if err := wantLine("L\t101\tSTORM\n"); err != nil {
			serverErr <- err
			return
		}
		if _, err := server.Write([]byte("L\tKEY=game-token\tGAMEHOST=127.0.0.1\tGAMEPORT=4900\n")); err != nil {
			serverErr <- err
			return
		}
		serverErr <- nil
	}()

	entry, err := runSGE(context.Background(), client, creds, false, 200*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if err := <-serverErr; err != nil {
		t.Fatal(err)
	}
	if entry.host != "127.0.0.1" || entry.port != 4900 || entry.key != "game-token" || !entry.premium {
		t.Fatalf("entry = %#v", entry)
	}
}

func TestSGETLSProtocolTranscript(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()

	creds := Credentials{Account: "test_account", Password: "password", Character: "hErO"}
	serverErr := make(chan error, 1)
	go func() {
		reader := bufio.NewReader(server)
		wantLine := func(want string) error {
			got, err := reader.ReadString('\n')
			if err != nil {
				return err
			}
			if got != want {
				return fmt.Errorf("SGE command = %q, want %q", got, want)
			}
			return nil
		}
		if err := wantLine("K\n"); err != nil {
			serverErr <- err
			return
		}
		key := bytes.Repeat([]byte{'K'}, 32)
		if _, err := server.Write(key); err != nil {
			serverErr <- err
			return
		}
		auth, err := reader.ReadBytes('\n')
		if err != nil {
			serverErr <- err
			return
		}
		encoded, _ := transformPassword(creds.Password, key)
		wantAuth := append([]byte("A\tTEST_ACCOUNT\t"), encoded...)
		wantAuth = append(wantAuth, '\n')
		if !bytes.Equal(auth, wantAuth) {
			serverErr <- errors.New("unexpected TLS auth bytes")
			return
		}
		for _, step := range []struct {
			response string
			next     string
		}{
			{response: "A\tOK\tVALID", next: "M\n"},
			{response: "M\tDR", next: "G\tDR\n"},
			{response: "G\tPREMIUM", next: "C\n"},
			{response: "C\ta\tb\tc\td\t101\tHero", next: "L\t101\tSTORM\n"},
		} {
			if _, err := server.Write([]byte(step.response)); err != nil {
				serverErr <- err
				return
			}
			if err := wantLine(step.next); err != nil {
				serverErr <- err
				return
			}
		}
		_, err = server.Write([]byte("L\tKEY=game-token\tGAMEHOST=127.0.0.1\tGAMEPORT=4900"))
		serverErr <- err
	}()

	entry, err := runSGE(context.Background(), client, creds, true, time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}
	if err := <-serverErr; err != nil {
		t.Fatal(err)
	}
	if entry.host != "127.0.0.1" || entry.port != 4900 || entry.key != "game-token" || !entry.premium {
		t.Fatalf("entry = %#v", entry)
	}
}

func TestSGEAuthFailuresDoNotLeakSecrets(t *testing.T) {
	t.Parallel()

	for _, response := range []string{"A\tNO\tPASSWORD", "A\tNO\tUNKNOWN ACCOUNT", "not-an-auth-response"} {
		err := validateAuthResponse(response)
		if err == nil {
			t.Fatalf("validateAuthResponse(%q) succeeded", response)
		}
		if strings.Contains(err.Error(), response) || strings.Contains(strings.ToLower(err.Error()), "account") {
			t.Fatalf("error leaked response or account data: %v", err)
		}
		var authErr *sgeAuthError
		if !errors.As(err, &authErr) {
			t.Fatalf("error type = %T, want *sgeAuthError", err)
		}
	}
}

func TestSGEAuthenticationWritesNoProcessOutput(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	serverDone := make(chan struct{})
	go func() {
		defer close(serverDone)
		reader := bufio.NewReader(server)
		_, _ = reader.ReadString('\n')
		_, _ = server.Write(append(bytes.Repeat([]byte{'K'}, 32), '\n'))
		_, _ = reader.ReadBytes('\n')
		_, _ = server.Write([]byte("A\t\tPASSWORD\n"))
	}()

	stdout, err := os.CreateTemp(t.TempDir(), "stdout")
	if err != nil {
		t.Fatal(err)
	}
	stderr, err := os.CreateTemp(t.TempDir(), "stderr")
	if err != nil {
		t.Fatal(err)
	}
	oldStdout, oldStderr := os.Stdout, os.Stderr
	os.Stdout, os.Stderr = stdout, stderr
	defer func() {
		os.Stdout, os.Stderr = oldStdout, oldStderr
	}()
	_, authErr := runSGE(context.Background(), client, Credentials{Account: "sensitive-account", Password: "sensitive-password", Character: "Hero"}, false, time.Second)
	os.Stdout, os.Stderr = oldStdout, oldStderr
	_ = stdout.Close()
	_ = stderr.Close()
	<-serverDone
	if authErr == nil {
		t.Fatal("rejected authentication succeeded")
	}
	for _, path := range []string{stdout.Name(), stderr.Name()} {
		output, err := os.ReadFile(path)
		if err != nil {
			t.Fatal(err)
		}
		if len(output) != 0 {
			t.Fatalf("authentication wrote process output to %s", filepath.Base(path))
		}
	}
}

func TestTLSFallbackPolicy(t *testing.T) {
	t.Parallel()

	plain := sgeEntry{host: "plain", port: 1}
	for _, tt := range []struct {
		name      string
		tlsErr    error
		cancel    bool
		wantPlain bool
	}{
		{name: "transport failure", tlsErr: io.ErrUnexpectedEOF, wantPlain: true},
		{name: "character not found", tlsErr: errCharacterNotFound, wantPlain: true},
		{name: "invalid port", tlsErr: errors.New("SGE returned an invalid game port"), wantPlain: true},
		{name: "authentication failure", tlsErr: &sgeAuthError{kind: "password"}},
		{name: "incomplete login", tlsErr: &sgeAuthError{kind: "malformed"}},
		{name: "unknown account", tlsErr: &sgeAuthError{kind: "unknown"}},
		{name: "problem response", tlsErr: &sgeAuthError{kind: "problem-2"}},
		{name: "caller cancellation", tlsErr: context.Canceled, cancel: true},
	} {
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			if tt.cancel {
				var cancel context.CancelFunc
				ctx, cancel = context.WithCancel(ctx)
				cancel()
			}
			calls := 0
			got, err := withTLSFallback(ctx, func(_ context.Context, tls bool) (sgeEntry, error) {
				calls++
				if tls {
					return sgeEntry{}, tt.tlsErr
				}
				return plain, nil
			})
			if tt.wantPlain {
				if err != nil || got != plain || calls != 2 {
					t.Fatalf("got %#v, err %v, calls %d", got, err, calls)
				}
			} else if err == nil || calls != 1 {
				t.Fatalf("err = %v, calls = %d", err, calls)
			}
		})
	}
}

func TestSGECertificatePin(t *testing.T) {
	t.Parallel()

	pinned, err := hex.DecodeString(sgeCertSHA256Pin)
	if err != nil {
		t.Fatal(err)
	}
	if !certificateHashPinned(pinned) {
		t.Fatal("configured certificate hash did not match its pin")
	}
	pinned[0] ^= 0xff
	if certificateHashPinned(pinned) {
		t.Fatal("mismatched certificate hash passed")
	}
}

func TestSGETLSWrapperRejectsUnpinnedCertificate(t *testing.T) {
	server := httptest.NewUnstartedServer(http.NotFoundHandler())
	server.Config.ErrorLog = log.New(io.Discard, "", 0)
	server.StartTLS()
	defer server.Close()

	conn, err := net.Dial("tcp", server.Listener.Addr().String())
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := wrapSGETLS(context.Background(), conn, "sge.invalid"); err == nil || !strings.Contains(err.Error(), "certificate pin mismatch") {
		t.Fatalf("wrapSGETLS error = %v, want certificate pin mismatch", err)
	}
}

func TestSGETLSWrapperHonorsCanceledContext(t *testing.T) {
	client, server := net.Pipe()
	defer client.Close()
	defer server.Close()
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := wrapSGETLS(ctx, client, "sge.invalid"); !errors.Is(err, context.Canceled) {
		t.Fatalf("wrapSGETLS error = %v, want context cancellation", err)
	}
}

func TestSGECancellationClosesBlockedTransport(t *testing.T) {
	client, server := net.Pipe()
	defer server.Close()
	started := make(chan struct{})
	go func() {
		reader := bufio.NewReader(server)
		_, _ = reader.ReadString('\n')
		close(started)
		_, _ = io.Copy(io.Discard, reader)
	}()

	cfg := sgeDialConfig{
		host:      "sge.invalid",
		tlsPort:   1,
		plainPort: 2,
		dial: func(context.Context, string, string) (net.Conn, error) {
			return client, nil
		},
		tlsWrap: func(_ context.Context, conn net.Conn, _ string) (net.Conn, error) {
			return conn, nil
		},
	}
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := authenticateSGE(ctx, Credentials{Account: "account", Password: "password", Character: "Hero"}, cfg)
		done <- err
	}()
	<-started
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("canceled authentication error = %v", err)
		}
	case <-time.After(time.Second):
		t.Fatal("canceled authentication left its transport blocked")
	}
}

type timeoutError struct{}

func (timeoutError) Error() string   { return "timeout" }
func (timeoutError) Timeout() bool   { return true }
func (timeoutError) Temporary() bool { return true }

type readDeadlineRecord struct {
	deadline time.Time
}

type delayedTLSResponseConn struct {
	firstByteAt time.Time
	response    []byte
	deadline    time.Time
	deadlines   []readDeadlineRecord
	reads       int
}

func (c *delayedTLSResponseConn) Read(buffer []byte) (int, error) {
	if c.reads == 0 {
		c.reads++
		if !c.deadline.IsZero() && c.deadline.Before(c.firstByteAt) {
			return 0, timeoutError{}
		}
		return copy(buffer, c.response), nil
	}
	return 0, timeoutError{}
}

func (c *delayedTLSResponseConn) Write(buffer []byte) (int, error) { return len(buffer), nil }
func (c *delayedTLSResponseConn) Close() error                     { return nil }
func (c *delayedTLSResponseConn) LocalAddr() net.Addr              { return fakeAddr("local") }
func (c *delayedTLSResponseConn) RemoteAddr() net.Addr             { return fakeAddr("remote") }
func (c *delayedTLSResponseConn) SetDeadline(deadline time.Time) error {
	c.deadline = deadline
	return nil
}
func (c *delayedTLSResponseConn) SetReadDeadline(deadline time.Time) error {
	c.deadline = deadline
	c.deadlines = append(c.deadlines, readDeadlineRecord{deadline: deadline})
	return nil
}
func (c *delayedTLSResponseConn) SetWriteDeadline(time.Time) error { return nil }
