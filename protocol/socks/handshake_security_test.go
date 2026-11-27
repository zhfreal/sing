package socks_test

import (
	"bytes"
	"context"
	"net"
	"net/netip"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/metacubex/sing/common/auth"
	"github.com/metacubex/sing/common/buf"
	"github.com/metacubex/sing/common/bufio"
	M "github.com/metacubex/sing/common/metadata"
	N "github.com/metacubex/sing/common/network"
	"github.com/metacubex/sing/protocol/socks"
)

type noopSocksHandler struct{}

func (noopSocksHandler) NewConnection(ctx context.Context, conn net.Conn, metadata M.Metadata) error {
	return conn.Close()
}

func (noopSocksHandler) NewPacketConnection(ctx context.Context, conn N.PacketConn, metadata M.Metadata) error {
	return conn.Close()
}

func TestSocks5AuthErrorRedactsPassword(t *testing.T) {
	au := auth.NewAuthenticator([]auth.User{{Username: "u", Password: "good"}})
	c1, c2 := net.Pipe()
	defer c1.Close()
	defer c2.Close()

	errCh := make(chan error, 1)
	go func() {
		errCh <- socks.HandleConnection(context.Background(), c2, au, noopSocksHandler{}, M.Metadata{})
	}()

	go func() {
		// Greeting: VER=5, NMETHODS=1, METHOD=2
		_, _ = c1.Write([]byte{5, 1, 2})
		var rep [2]byte
		_, _ = c1.Read(rep[:])
		// Username/Password auth with wrong password "my-secret-pw"
		_, _ = c1.Write(append([]byte{1, 1, 'u', 12}, []byte("my-secret-pw")...))
		_, _ = c1.Read(rep[:])
	}()

	err := <-errCh
	if err == nil {
		t.Fatalf("expected authentication error")
	}
	if strings.Contains(err.Error(), "my-secret-pw") {
		t.Fatalf("SOCKS5 error leaked plaintext password: %v", err)
	}
}

func TestServerPacketConnSourceFilterAndFragDrop(t *testing.T) {
	udpSrv, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("ListenUDP: %v", err)
	}
	defer udpSrv.Close()

	allowedIP := netip.MustParseAddr("127.0.0.1")
	spoofIP := netip.MustParseAddr("127.0.0.2")

	spc := bufio.NewServerPacketConnWithSource(udpSrv, allowedIP)
	assoc := socks.NewAssociatePacketConn(spc, M.Socksaddr{}, nil)

	// Send spoofed packet from 127.0.0.2 (should be dropped), then valid packet with FRAG=1 (should be rejected),
	// then valid packet with FRAG=0 from 127.0.0.1.
	spoofClient, err := net.DialUDP("udp4", &net.UDPAddr{IP: spoofIP.AsSlice(), Port: 0}, udpSrv.LocalAddr().(*net.UDPAddr))
	if err == nil {
		defer spoofClient.Close()
		_, _ = spoofClient.Write([]byte{0, 0, 0, 1, 8, 8, 8, 8, 0, 53, 'b', 'a', 'd'})
	}

	validClient, err := net.DialUDP("udp4", &net.UDPAddr{IP: allowedIP.AsSlice(), Port: 0}, udpSrv.LocalAddr().(*net.UDPAddr))
	if err != nil {
		t.Fatalf("DialUDP validClient: %v", err)
	}
	defer validClient.Close()

	// FRAG=1 packet from allowed IP -> ErrInvalidPacket
	_, _ = validClient.Write([]byte{0, 0, 1, 1, 8, 8, 8, 8, 0, 53, 'f'})
	_ = udpSrv.SetReadDeadline(time.Now().Add(time.Second))
	b := buf.NewPacket()
	defer b.Release()
	if _, rErr := assoc.ReadPacket(b); rErr != socks.ErrInvalidPacket {
		t.Fatalf("expected ErrInvalidPacket for FRAG=1, got %v", rErr)
	}

	// Valid FRAG=0 packet from allowed IP
	b.Reset()
	_, _ = validClient.Write([]byte{0, 0, 0, 1, 8, 8, 8, 8, 0, 53, 'o', 'k'})
	dest, rErr := assoc.ReadPacket(b)
	if rErr != nil {
		t.Fatalf("ReadPacket valid: %v", rErr)
	}
	if !bytes.Equal(b.Bytes(), []byte("ok")) || dest.Port != 53 {
		t.Fatalf("unexpected packet payload=%q dest=%v", b.Bytes(), dest)
	}

	// Concurrent ReadPacket / RemoteAddr race check
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			_ = assoc.RemoteAddr()
		}
	}()
	go func() {
		defer wg.Done()
		for i := 0; i < 10; i++ {
			_ = spc.RemoteAddr()
		}
	}()
	wg.Wait()
}

func TestServerPacketConnReadBufferPreservesHeadroomOnSpoofedDrop(t *testing.T) {
	udpSrv, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: 0})
	if err != nil {
		t.Fatalf("ListenUDP: %v", err)
	}
	defer udpSrv.Close()

	allowedIP := netip.MustParseAddr("127.0.0.1")
	spoofIP := netip.MustParseAddr("127.0.0.2")
	spc := bufio.NewServerPacketConnWithSource(udpSrv, allowedIP)

	spoofClient, err := net.DialUDP("udp4", &net.UDPAddr{IP: spoofIP.AsSlice(), Port: 0}, udpSrv.LocalAddr().(*net.UDPAddr))
	if err == nil {
		defer spoofClient.Close()
		_, _ = spoofClient.Write([]byte("spoofed-payload"))
	}
	validClient, err := net.DialUDP("udp4", &net.UDPAddr{IP: allowedIP.AsSlice(), Port: 0}, udpSrv.LocalAddr().(*net.UDPAddr))
	if err != nil {
		t.Fatalf("DialUDP validClient: %v", err)
	}
	defer validClient.Close()
	_, _ = validClient.Write([]byte("legit"))

	_ = udpSrv.SetReadDeadline(time.Now().Add(time.Second))
	b := buf.NewPacket()
	defer b.Release()
	const frontHeadroom = 16
	const rearHeadroom = 8
	b.Resize(frontHeadroom, 0)
	b.Reserve(rearHeadroom)

	if err := spc.ReadBuffer(b); err != nil {
		t.Fatalf("ReadBuffer: %v", err)
	}
	if b.Start() != frontHeadroom {
		t.Fatalf("FrontHeadroom wiped: got Start()=%d, want %d", b.Start(), frontHeadroom)
	}
	// OverCap must not panic (RearHeadroom preserved).
	b.OverCap(rearHeadroom)
	if !bytes.Equal(b.Bytes(), []byte("legit")) {
		t.Fatalf("unexpected payload %q", b.Bytes())
	}
}
