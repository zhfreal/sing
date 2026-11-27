package http_test

import (
	std_bufio "bufio"
	"bytes"
	"context"
	"encoding/base64"
	"io"
	"net"
	std_http "net/http"
	"strings"
	"testing"

	"github.com/metacubex/sing/common/auth"
	M "github.com/metacubex/sing/common/metadata"
	"github.com/metacubex/sing/protocol/http"
)

type captureHandler struct {
	gotMetadata M.Metadata
	gotPayload  bytes.Buffer
	done        chan struct{}
}

func (h *captureHandler) NewConnection(ctx context.Context, conn net.Conn, metadata M.Metadata) error {
	defer close(h.done)
	h.gotMetadata = metadata
	_, _ = io.Copy(&h.gotPayload, conn)
	return conn.Close()
}

func TestUpgradeStripsProxyAuthorizationAndStdBase64AndXFF(t *testing.T) {
	// Password produces '+' / '/' in StdEncoding (">?>" -> "Pj8+").
	username := "user"
	password := "?>?"
	cred := base64.StdEncoding.EncodeToString([]byte(username + ":" + password))
	if !strings.ContainsAny(cred, "+/") {
		t.Fatalf("test credential %q should exercise + or /", cred)
	}

	au := auth.NewAuthenticator([]auth.User{{Username: username, Password: password}})
	handler := &captureHandler{done: make(chan struct{})}

	c1, c2 := net.Pipe()
	errCh := make(chan error, 1)
	go func() {
		meta := M.Metadata{Source: M.ParseSocksaddr("198.51.100.10:12345")}
		errCh <- http.HandleConnection(context.Background(), c2, std_bufio.NewReader(c2), au, handler, meta)
	}()

	req := "GET http://example.com/ws HTTP/1.1\r\n" +
		"Host: example.com\r\n" +
		"Connection: Upgrade\r\n" +
		"Upgrade: websocket\r\n" +
		"Proxy-Connection: keep-alive\r\n" +
		"Proxy-Authorization: Basic " + cred + "\r\n" +
		"X-Forwarded-For: 127.0.0.1\r\n\r\n"
	if _, err := io.WriteString(c1, req); err != nil {
		t.Fatalf("write request: %v", err)
	}
	_ = c1.Close()
	<-errCh
	<-handler.done

	forwarded := handler.gotPayload.String()
	if strings.Contains(strings.ToLower(forwarded), "proxy-authorization") {
		t.Fatalf("Proxy-Authorization leaked in forwarded Upgrade request: %q", forwarded)
	}
	if strings.Contains(strings.ToLower(forwarded), "proxy-connection") {
		t.Fatalf("Proxy-Connection leaked in forwarded Upgrade request: %q", forwarded)
	}
	if handler.gotMetadata.Source.AddrString() != "198.51.100.10" {
		t.Fatalf("SourceAddress trusted spoofed X-Forwarded-For: got %s", handler.gotMetadata.Source)
	}

	// Verify failed auth error omits plaintext password.
	c3, c4 := net.Pipe()
	errCh2 := make(chan error, 1)
	go func() {
		errCh2 <- http.HandleConnection(context.Background(), c4, std_bufio.NewReader(c4), au, &captureHandler{done: make(chan struct{})}, M.Metadata{})
	}()
	badCred := base64.StdEncoding.EncodeToString([]byte("user:secret-bad-pass"))
	go func() {
		_, _ = io.WriteString(c3, "CONNECT example.com:443 HTTP/1.1\r\nProxy-Authorization: Basic "+badCred+"\r\n\r\n")
		_, _ = io.Copy(io.Discard, c3)
		_ = c3.Close()
	}()
	authErr := <-errCh2
	if authErr == nil {
		t.Fatalf("expected auth error")
	}
	if strings.Contains(authErr.Error(), "secret-bad-pass") {
		t.Fatalf("auth error leaked plaintext password: %v", authErr)
	}
	_ = std_http.MethodGet
}
