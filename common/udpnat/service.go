package udpnat

import (
	"context"
	"io"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"github.com/metacubex/sing/common"
	"github.com/metacubex/sing/common/buf"
	"github.com/metacubex/sing/common/cache"
	E "github.com/metacubex/sing/common/exceptions"
	M "github.com/metacubex/sing/common/metadata"
	N "github.com/metacubex/sing/common/network"
	"github.com/metacubex/sing/common/pipe"
)

const defaultMaxNATEntries = 4096

type Handler interface {
	N.UDPConnectionHandler
	E.Handler
}

type Service[K comparable] struct {
	nat     *cache.LruCache[K, *conn]
	handler Handler
}

func New[K comparable](maxAge int64, handler Handler) *Service[K] {
	return NewWithSize[K](maxAge, defaultMaxNATEntries, handler)
}

func NewWithSize[K comparable](maxAge int64, maxSize int, handler Handler) *Service[K] {
	options := []cache.Option[K, *conn]{
		cache.WithAge[K, *conn](maxAge),
		cache.WithUpdateAgeOnGet[K, *conn](),
		cache.WithEvict[K, *conn](func(key K, conn *conn) {
			conn.Close()
		}),
	}
	if maxSize > 0 {
		options = append(options, cache.WithSize[K, *conn](maxSize))
	}
	return &Service[K]{
		nat:     cache.New(options...),
		handler: handler,
	}
}

func (s *Service[T]) WriteIsThreadUnsafe() {
}

func (s *Service[T]) NewPacketDirect(ctx context.Context, key T, conn N.PacketConn, buffer *buf.Buffer, metadata M.Metadata) {
	s.NewContextPacket(ctx, key, buffer, metadata, func(natConn N.PacketConn) (context.Context, N.PacketWriter) {
		return ctx, &DirectBackWriter{conn, natConn}
	})
}

type DirectBackWriter struct {
	Source N.PacketConn
	Nat    N.PacketConn
}

func (w *DirectBackWriter) WritePacket(buffer *buf.Buffer, addr M.Socksaddr) error {
	return w.Source.WritePacket(buffer, M.SocksaddrFromNet(w.Nat.LocalAddr()))
}

func (w *DirectBackWriter) Upstream() any {
	return w.Source
}

func (s *Service[T]) NewPacket(ctx context.Context, key T, buffer *buf.Buffer, metadata M.Metadata, init func(natConn N.PacketConn) N.PacketWriter) {
	s.NewContextPacket(ctx, key, buffer, metadata, func(natConn N.PacketConn) (context.Context, N.PacketWriter) {
		return ctx, init(natConn)
	})
}

func (s *Service[T]) NewContextPacket(ctx context.Context, key T, buffer *buf.Buffer, metadata M.Metadata, init func(natConn N.PacketConn) (context.Context, N.PacketWriter)) {
	c, loaded := s.nat.LoadOrStore(key, func() *conn {
		c := &conn{
			data:         make(chan packet, 64),
			remoteAddr:   metadata.Destination,
			readDeadline: pipe.MakeDeadline(),
		}
		c.localAddr.Store(&metadata.Source)
		c.ctx, c.cancel = common.ContextWithCancelCause(ctx)
		return c
	})
	if !loaded {
		initCtx, source := init(c)
		c.sourceMu.Lock()
		c.source = source
		alreadyClosed := c.closed
		c.sourceMu.Unlock()
		if alreadyClosed {
			if sourceCloser, sourceIsCloser := source.(io.Closer); sourceIsCloser {
				_ = sourceCloser.Close()
			}
		}
		go func() {
			err := s.handler.NewPacketConnection(initCtx, c, metadata)
			if err != nil {
				s.handler.NewError(initCtx, err)
			}
			c.Close()
			s.nat.Delete(key)
		}()
	} else {
		addr := metadata.Source
		c.localAddr.Store(&addr)
	}
	if common.Done(c.ctx) {
		s.nat.Delete(key)
		if !common.Done(ctx) {
			s.NewContextPacket(ctx, key, buffer, metadata, init)
		} else {
			buffer.Release()
		}
		return
	}
	select {
	case c.data <- packet{
		data:        buffer,
		destination: metadata.Destination,
	}:
	case <-c.ctx.Done():
		buffer.Release()
	default:
		// Drop packet instead of stalling the shared dispatcher when a NAT entry backs up.
		buffer.Release()
	}
}

type packet struct {
	data        *buf.Buffer
	destination M.Socksaddr
}

var _ N.PacketConn = (*conn)(nil)

type conn struct {
	ctx             context.Context
	cancel          common.ContextCancelCauseFunc
	data            chan packet
	localAddr       atomic.Pointer[M.Socksaddr]
	remoteAddr      M.Socksaddr
	sourceMu        sync.Mutex
	source          N.PacketWriter
	closed          bool
	readDeadline    pipe.Deadline
	readWaitOptions N.ReadWaitOptions
}

func (c *conn) ReadPacket(buffer *buf.Buffer) (addr M.Socksaddr, err error) {
	select {
	case p := <-c.data:
		_, err = buffer.ReadOnceFrom(p.data)
		p.data.Release()
		return p.destination, err
	case <-c.ctx.Done():
		return M.Socksaddr{}, io.ErrClosedPipe
	case <-c.readDeadline.Wait():
		return M.Socksaddr{}, os.ErrDeadlineExceeded
	}
}

func (c *conn) WritePacket(buffer *buf.Buffer, destination M.Socksaddr) error {
	c.sourceMu.Lock()
	source := c.source
	c.sourceMu.Unlock()
	if source == nil {
		buffer.Release()
		return io.ErrClosedPipe
	}
	return source.WritePacket(buffer, destination)
}

func (c *conn) Close() error {
	select {
	case <-c.ctx.Done():
	default:
		c.cancel(net.ErrClosed)
	}
	c.sourceMu.Lock()
	c.closed = true
	source := c.source
	c.sourceMu.Unlock()
	for {
		select {
		case p := <-c.data:
			p.data.Release()
		default:
			if sourceCloser, sourceIsCloser := source.(io.Closer); sourceIsCloser {
				return sourceCloser.Close()
			}
			return nil
		}
	}
}

func (c *conn) LocalAddr() net.Addr {
	if addr := c.localAddr.Load(); addr != nil {
		return *addr
	}
	return M.Socksaddr{}
}

func (c *conn) RemoteAddr() net.Addr {
	return c.remoteAddr
}

func (c *conn) SetDeadline(t time.Time) error {
	return os.ErrInvalid
}

func (c *conn) SetReadDeadline(t time.Time) error {
	c.readDeadline.Set(t)
	return nil
}

func (c *conn) SetWriteDeadline(t time.Time) error {
	return os.ErrInvalid
}

func (c *conn) NeedAdditionalReadDeadline() bool {
	return true
}

func (c *conn) Upstream() any {
	c.sourceMu.Lock()
	source := c.source
	c.sourceMu.Unlock()
	return source
}
