package ntp

import (
	"bytes"
	"context"
	"encoding/binary"
	"net"
	"testing"
	"time"

	M "github.com/metacubex/sing/common/metadata"
	N "github.com/metacubex/sing/common/network"
)

type mockNTPConn struct {
	net.Conn
	respond func(req msg) msg
	reply   bytes.Reader
}

func (m *mockNTPConn) Read(b []byte) (int, error)  { return m.reply.Read(b) }
func (m *mockNTPConn) Write(b []byte) (int, error) {
	var req msg
	if err := binary.Read(bytes.NewReader(b), binary.BigEndian, &req); err != nil {
		return 0, err
	}
	resp := m.respond(req)
	var out bytes.Buffer
	_ = binary.Write(&out, binary.BigEndian, resp)
	m.reply = *bytes.NewReader(out.Bytes())
	return len(b), nil
}
func (m *mockNTPConn) Close() error                       { return nil }
func (m *mockNTPConn) LocalAddr() net.Addr                { return nil }
func (m *mockNTPConn) RemoteAddr() net.Addr               { return nil }
func (m *mockNTPConn) SetDeadline(t time.Time) error      { return nil }
func (m *mockNTPConn) SetReadDeadline(t time.Time) error  { return nil }
func (m *mockNTPConn) SetWriteDeadline(t time.Time) error { return nil }

type mockNTPDialer struct {
	respond func(req msg) msg
}

func (d mockNTPDialer) DialContext(ctx context.Context, network string, destination M.Socksaddr) (net.Conn, error) {
	return &mockNTPConn{respond: d.respond}, nil
}

func (d mockNTPDialer) ListenPacket(ctx context.Context, destination M.Socksaddr) (net.PacketConn, error) {
	return nil, nil
}

func TestExchangeRejectsSpoofedAndInvalidResponses(t *testing.T) {
	now := toNtpTime(time.Now())
	validBase := func(req msg) msg {
		var resp msg
		resp.setMode(server)
		resp.setVersion(defaultNtpVersion)
		resp.setLeap(LeapNoWarning)
		resp.Stratum = 2
		resp.ReferenceTime = now
		resp.OriginTime = req.TransmitTime
		resp.ReceiveTime = now
		resp.TransmitTime = now
		return resp
	}

	// 1. Mismatched OriginTime (spoofed reply)
	if _, err := Exchange(context.Background(), mockNTPDialer{respond: func(req msg) msg {
		r := validBase(req)
		r.OriginTime = req.TransmitTime ^ 1
		return r
	}}, M.Socksaddr{Fqdn: "ntp.example", Port: 123}); err == nil {
		t.Fatalf("expected error on OriginTime mismatch")
	}

	// 2. Non-server mode
	if _, err := Exchange(context.Background(), mockNTPDialer{respond: func(req msg) msg {
		r := validBase(req)
		r.setMode(client)
		return r
	}}, M.Socksaddr{Fqdn: "ntp.example", Port: 123}); err == nil {
		t.Fatalf("expected error on non-server mode")
	}

	// 3. Stratum 0 / Kiss-of-Death (Validate failure)
	if _, err := Exchange(context.Background(), mockNTPDialer{respond: func(req msg) msg {
		r := validBase(req)
		r.Stratum = 0
		return r
	}}, M.Socksaddr{Fqdn: "ntp.example", Port: 123}); err == nil {
		t.Fatalf("expected error on stratum 0")
	}

	// 4. Valid reply succeeds
	if _, err := Exchange(context.Background(), mockNTPDialer{respond: validBase}, M.Socksaddr{Fqdn: "ntp.example", Port: 123}); err != nil {
		t.Fatalf("expected valid reply to succeed, got: %v", err)
	}
	_ = N.NetworkUDP
}
