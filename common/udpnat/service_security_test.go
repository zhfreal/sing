package udpnat_test

import (
	"context"
	"net/netip"
	"sync"
	"testing"
	"time"

	"github.com/metacubex/sing/common/buf"
	M "github.com/metacubex/sing/common/metadata"
	N "github.com/metacubex/sing/common/network"
	"github.com/metacubex/sing/common/udpnat"
)

type stalledHandler struct {
	blockCh chan struct{}
}

func (h *stalledHandler) NewPacketConnection(ctx context.Context, conn N.PacketConn, metadata M.Metadata) error {
	<-h.blockCh
	return nil
}

func (h *stalledHandler) NewError(ctx context.Context, err error) {}

type noopWriter struct{}

func (noopWriter) WritePacket(buffer *buf.Buffer, destination M.Socksaddr) error {
	buffer.Release()
	return nil
}

func TestUDPServiceNonBlockingOnFullOrClosedEntry(t *testing.T) {
	h := &stalledHandler{blockCh: make(chan struct{})}
	svc := udpnat.NewWithSize[uint64](60, 4, h)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	meta := M.Metadata{
		Source:      M.SocksaddrFrom(netip.MustParseAddr("127.0.0.1"), 10000),
		Destination: M.SocksaddrFrom(netip.MustParseAddr("8.8.8.8"), 53),
	}

	done := make(chan struct{})
	go func() {
		defer close(done)
		var wg sync.WaitGroup
		for k := uint64(0); k < 8; k++ {
			wg.Add(1)
			go func(key uint64) {
				defer wg.Done()
				for i := 0; i < 100; i++ {
					b := buf.NewPacket()
					_, _ = b.WriteString("pkt")
					svc.NewPacket(ctx, key, b, meta, func(natConn N.PacketConn) N.PacketWriter {
						return noopWriter{}
					})
				}
			}(k)
		}
		wg.Wait()
	}()

	select {
	case <-done:
	case <-time.After(2 * time.Second):
		t.Fatalf("NewPacket deadlocked on full/evicted NAT entries")
	}
	close(h.blockCh)
}
