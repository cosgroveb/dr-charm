package dragonrealms

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

const (
	defaultSGEHost       = "eaccess.play.net"
	defaultSGETLSPort    = 7910
	defaultSGEPlainPort  = 7900
	sgeTLSAttemptTimeout = 8 * time.Second
	sgeTLSResponseIdle   = 200 * time.Millisecond
	sgeCertSHA256Pin     = "10B737E661987D15BC5C8245E3F8B78291D41ED8ABC76672ECB02FE78ED0218A"
)

type sgeCharacter struct {
	code string
	name string
}

type sgeEntry struct {
	host    string
	port    int
	key     string
	premium bool
}

type sgeDialConfig struct {
	host      string
	tlsPort   int
	plainPort int
	dial      func(context.Context, string, string) (net.Conn, error)
	tlsWrap   func(context.Context, net.Conn, string) (net.Conn, error)
}

func defaultSGEDialConfig() sgeDialConfig {
	dialer := &net.Dialer{Timeout: 15 * time.Second}
	return sgeDialConfig{
		host:      defaultSGEHost,
		tlsPort:   defaultSGETLSPort,
		plainPort: defaultSGEPlainPort,
		dial:      dialer.DialContext,
		tlsWrap:   wrapSGETLS,
	}
}

func validateCredentials(credentials Credentials) error {
	for _, value := range []string{credentials.Account, credentials.Password, credentials.Character} {
		if value == "" {
			return &sgeAuthError{kind: "invalid"}
		}
		for i := 0; i < len(value); i++ {
			if value[i] < 32 || value[i] > 126 {
				return &sgeAuthError{kind: "invalid"}
			}
		}
	}
	return nil
}

func asciiUpper(value string) string {
	upper := []byte(value)
	for i, b := range upper {
		if b >= 'a' && b <= 'z' {
			upper[i] = b - ('a' - 'A')
		}
	}
	return string(upper)
}

func transformPassword(password string, key []byte) ([]byte, error) {
	if len(key) != 32 {
		return nil, errors.New("invalid SGE key length")
	}
	transformed := make([]byte, len(password))
	for i := range password {
		transformed[i] = ((password[i] - 32) ^ key[i%len(key)]) + 32
	}
	return transformed, nil
}

func readSGEKey(reader *bufio.Reader, useTLS bool) ([]byte, error) {
	key := make([]byte, 32)
	if _, err := io.ReadFull(reader, key); err != nil {
		return nil, errors.New("SGE key response was incomplete")
	}
	if useTLS {
		return key, nil
	}
	newline, err := reader.ReadByte()
	if err != nil || newline != '\n' {
		return nil, errors.New("SGE key response had invalid framing")
	}
	return key, nil
}

func parseCharacters(response string) ([]sgeCharacter, error) {
	fields := strings.Split(strings.TrimSpace(response), "\t")
	if len(fields) < 7 || (len(fields)-5)%2 != 0 {
		return nil, errors.New("SGE character response was malformed")
	}
	characters := make([]sgeCharacter, 0, (len(fields)-5)/2)
	for i := 5; i+1 < len(fields); i += 2 {
		if fields[i] == "" || fields[i+1] == "" {
			return nil, errors.New("SGE character response was malformed")
		}
		characters = append(characters, sgeCharacter{code: fields[i], name: fields[i+1]})
	}
	return characters, nil
}

func validateAuthResponse(response string) error {
	fields := strings.Split(strings.TrimSpace(response), "\t")
	if len(fields) < 3 || !strings.EqualFold(fields[0], "A") {
		return &sgeAuthError{kind: "malformed"}
	}
	for _, field := range fields[2:] {
		switch {
		case strings.EqualFold(field, "PASSWORD"):
			return &sgeAuthError{kind: "password"}
		case strings.HasPrefix(strings.ToUpper(field), "UNKNOWN"):
			return &sgeAuthError{kind: "unknown"}
		}
	}
	return nil
}

func parseLoginResponse(response string) (sgeEntry, error) {
	fields := strings.Split(strings.TrimSpace(response), "\t")
	for _, field := range fields {
		problem := strings.TrimSpace(field)
		if !strings.HasPrefix(strings.ToUpper(problem), "PROBLEM") {
			continue
		}
		kind := "problem"
		parts := strings.Fields(problem)
		if len(parts) > 1 {
			switch parts[1] {
			case "1", "2", "3", "4":
				kind += "-" + parts[1]
			}
		}
		return sgeEntry{}, &sgeAuthError{kind: kind}
	}
	var entry sgeEntry
	for _, field := range fields {
		switch {
		case strings.HasPrefix(field, "KEY="):
			entry.key = strings.TrimPrefix(field, "KEY=")
		case strings.HasPrefix(field, "GAMEHOST="):
			entry.host = strings.TrimPrefix(field, "GAMEHOST=")
		case strings.HasPrefix(field, "GAMEPORT="):
			port, err := strconv.Atoi(strings.TrimPrefix(field, "GAMEPORT="))
			if err != nil || port < 1 || port > 65535 {
				return sgeEntry{}, errors.New("SGE returned an invalid game port")
			}
			entry.port = port
		}
	}
	if entry.key == "" || entry.host == "" || entry.port == 0 {
		return sgeEntry{}, &sgeAuthError{kind: "malformed"}
	}
	return entry, nil
}

func runSGE(ctx context.Context, conn net.Conn, credentials Credentials, useTLS bool, tlsIdle time.Duration) (sgeEntry, error) {
	if err := validateCredentials(credentials); err != nil {
		return sgeEntry{}, err
	}
	reader := bufio.NewReader(conn)
	if err := writeAll(conn, []byte("K\n")); err != nil {
		return sgeEntry{}, errors.New("could not request the SGE key")
	}
	key, err := readSGEKey(reader, useTLS)
	if err != nil {
		return sgeEntry{}, err
	}
	password, err := transformPassword(credentials.Password, key)
	if err != nil {
		return sgeEntry{}, err
	}
	auth := append([]byte("A\t"+asciiUpper(credentials.Account)+"\t"), password...)
	auth = append(auth, '\n')
	if err := writeAll(conn, auth); err != nil {
		return sgeEntry{}, errors.New("could not send SGE authentication")
	}
	authResponse, err := readSGEResponse(ctx, conn, reader, useTLS, tlsIdle)
	if err != nil {
		return sgeEntry{}, errors.New("could not read SGE authentication response")
	}
	if err := validateAuthResponse(authResponse); err != nil {
		return sgeEntry{}, err
	}

	if err := writeAll(conn, []byte("M\n")); err != nil {
		return sgeEntry{}, errors.New("could not request the SGE game list")
	}
	if _, err := readSGEResponse(ctx, conn, reader, useTLS, tlsIdle); err != nil {
		return sgeEntry{}, errors.New("could not read the SGE game list")
	}
	if err := writeAll(conn, []byte("G\tDR\n")); err != nil {
		return sgeEntry{}, errors.New("could not select DragonRealms")
	}
	gameResponse, err := readSGEResponse(ctx, conn, reader, useTLS, tlsIdle)
	if err != nil {
		return sgeEntry{}, errors.New("could not read the DragonRealms selection")
	}
	premium := false
	for _, field := range strings.Split(gameResponse, "\t") {
		if strings.EqualFold(strings.TrimSpace(field), "PREMIUM") {
			premium = true
			break
		}
	}

	if err := writeAll(conn, []byte("C\n")); err != nil {
		return sgeEntry{}, errors.New("could not request DragonRealms characters")
	}
	characterResponse, err := readSGEResponse(ctx, conn, reader, useTLS, tlsIdle)
	if err != nil {
		return sgeEntry{}, errors.New("could not read DragonRealms characters")
	}
	characters, err := parseCharacters(characterResponse)
	if err != nil {
		return sgeEntry{}, err
	}
	characterCode := ""
	for _, character := range characters {
		if strings.EqualFold(character.name, credentials.Character) {
			characterCode = character.code
			break
		}
	}
	if characterCode == "" {
		return sgeEntry{}, errCharacterNotFound
	}

	if err := writeAll(conn, []byte("L\t"+characterCode+"\tSTORM\n")); err != nil {
		return sgeEntry{}, errors.New("could not request the DragonRealms login")
	}
	loginResponse, err := readSGEResponse(ctx, conn, reader, useTLS, tlsIdle)
	if err != nil {
		return sgeEntry{}, errors.New("could not read the DragonRealms login")
	}
	entry, err := parseLoginResponse(loginResponse)
	if err != nil {
		return sgeEntry{}, err
	}
	entry.premium = premium
	return entry, nil
}

func readSGEResponse(ctx context.Context, conn net.Conn, reader *bufio.Reader, useTLS bool, idle time.Duration) (string, error) {
	if !useTLS {
		line, err := reader.ReadString('\n')
		if err != nil {
			return "", err
		}
		return strings.TrimSuffix(strings.TrimSuffix(line, "\n"), "\r"), nil
	}

	var response bytes.Buffer
	for {
		deadline := time.Time{}
		if response.Len() > 0 {
			deadline = time.Now().Add(idle)
		}
		if ctxDeadline, ok := ctx.Deadline(); ok && (deadline.IsZero() || ctxDeadline.Before(deadline)) {
			deadline = ctxDeadline
		}
		if err := conn.SetReadDeadline(deadline); err != nil {
			return "", err
		}
		chunk := make([]byte, 4096)
		n, err := reader.Read(chunk)
		if n > 0 {
			response.Write(chunk[:n])
		}
		if err == nil {
			continue
		}
		var netErr net.Error
		if errors.As(err, &netErr) && netErr.Timeout() && response.Len() > 0 {
			_ = conn.SetReadDeadline(time.Time{})
			return response.String(), nil
		}
		if errors.Is(err, io.EOF) && response.Len() > 0 {
			return response.String(), nil
		}
		return "", err
	}
}

func withTLSFallback(ctx context.Context, operation func(context.Context, bool) (sgeEntry, error)) (sgeEntry, error) {
	tlsCtx, cancel := context.WithTimeout(ctx, sgeTLSAttemptTimeout)
	entry, err := operation(tlsCtx, true)
	cancel()
	if err == nil {
		return entry, nil
	}
	if err := ctx.Err(); err != nil {
		return sgeEntry{}, err
	}
	var authErr *sgeAuthError
	if errors.As(err, &authErr) {
		return sgeEntry{}, err
	}
	return operation(ctx, false)
}

func authenticateSGE(ctx context.Context, credentials Credentials, cfg sgeDialConfig) (sgeEntry, error) {
	return withTLSFallback(ctx, func(attemptCtx context.Context, useTLS bool) (sgeEntry, error) {
		port := cfg.plainPort
		if useTLS {
			port = cfg.tlsPort
		}
		conn, err := cfg.dial(attemptCtx, "tcp", net.JoinHostPort(cfg.host, strconv.Itoa(port)))
		if err != nil {
			return sgeEntry{}, err
		}
		defer conn.Close()
		transport := conn
		stopClose := context.AfterFunc(attemptCtx, func() {
			_ = transport.Close()
		})
		defer stopClose()
		if useTLS {
			conn, err = cfg.tlsWrap(attemptCtx, conn, cfg.host)
			if err != nil {
				return sgeEntry{}, err
			}
		}
		return runSGE(attemptCtx, conn, credentials, useTLS, sgeTLSResponseIdle)
	})
}

func wrapSGETLS(ctx context.Context, conn net.Conn, host string) (net.Conn, error) {
	tlsConn := tls.Client(conn, &tls.Config{
		MinVersion:         tls.VersionTLS12,
		MaxVersion:         tls.VersionTLS13,
		ServerName:         host,
		InsecureSkipVerify: true,
		VerifyPeerCertificate: func(rawCerts [][]byte, _ [][]*x509.Certificate) error {
			if len(rawCerts) == 0 || !certificatePinned(rawCerts[0]) {
				return errors.New("SGE TLS certificate pin mismatch")
			}
			return nil
		},
	})
	if err := tlsConn.HandshakeContext(ctx); err != nil {
		return nil, err
	}
	return tlsConn, nil
}

func certificatePinned(der []byte) bool {
	sum := sha256.Sum256(der)
	return certificateHashPinned(sum[:])
}

func certificateHashPinned(hash []byte) bool {
	want, err := hex.DecodeString(sgeCertSHA256Pin)
	return err == nil && bytes.Equal(hash, want)
}

func writeAll(writer io.Writer, data []byte) error {
	for len(data) > 0 {
		n, err := writer.Write(data)
		if err != nil {
			return err
		}
		if n == 0 {
			return io.ErrShortWrite
		}
		data = data[n:]
	}
	return nil
}

func formatGameAddress(entry sgeEntry) string {
	return net.JoinHostPort(entry.host, fmt.Sprintf("%d", entry.port))
}
