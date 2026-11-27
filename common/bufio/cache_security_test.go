package bufio_test

import (
	"bytes"
	"errors"
	"io"
	"net"
	"testing"

	"github.com/metacubex/sing/common/buf"
	"github.com/metacubex/sing/common/bufio"
)

type failingWriter struct {
	err error
}

func (f failingWriter) Write(p []byte) (int, error) {
	return 0, f.err
}

func TestCachedConnWriteToErrorPropagationAndRelease(t *testing.T) {
	c1, c2 := net.Pipe()
	defer c1.Close()
	go func() {
		_, _ = c2.Write([]byte("tail"))
		_ = c2.Close()
	}()

	cached := buf.NewSize(32)
	_, _ = cached.WriteString("head")
	cc := bufio.NewCachedConn(c1, cached)

	expectedErr := errors.New("write failed")
	n, err := cc.WriteTo(failingWriter{err: expectedErr})
	if !errors.Is(err, expectedErr) {
		t.Fatalf("expected %v, got n=%d err=%v", expectedErr, n, err)
	}

	// Verify success path drains cached + upstream exactly once.
	c3, c4 := net.Pipe()
	defer c3.Close()
	go func() {
		_, _ = c4.Write([]byte("-tail"))
		_ = c4.Close()
	}()
	cached2 := buf.NewSize(32)
	_, _ = cached2.WriteString("head")
	cc2 := bufio.NewCachedConn(c3, cached2)
	var out bytes.Buffer
	n2, err2 := io.Copy(&out, cc2)
	if err2 != nil {
		t.Fatalf("io.Copy failed: %v", err2)
	}
	if n2 != 9 || out.String() != "head-tail" {
		t.Fatalf("got n=%d out=%q, want 9/'head-tail'", n2, out.String())
	}
}
