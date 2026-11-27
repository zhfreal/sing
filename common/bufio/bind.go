package bufio

import (
	"net"
	"net/netip"
	"sync/atomic"

	"github.com/metacubex/sing/common/buf"
	M "github.com/metacubex/sing/common/metadata"
	N "github.com/metacubex/sing/common/network"
)

type BindPacketConn interface {
	N.NetPacketConn
	net.Conn
}

type bindPacketConn struct {
	N.NetPacketConn
	addr net.Addr
}

func NewBindPacketConn(conn net.PacketConn, addr net.Addr) BindPacketConn {
	return &bindPacketConn{
		NewPacketConn(conn),
		addr,
	}
}

func (c *bindPacketConn) Read(b []byte) (n int, err error) {
	n, _, err = c.ReadFrom(b)
	return
}

func (c *bindPacketConn) Write(b []byte) (n int, err error) {
	return c.WriteTo(b, c.addr)
}

func (c *bindPacketConn) CreateReadWaiter() (N.ReadWaiter, bool) {
	readWaiter, isReadWaiter := CreatePacketReadWaiter(c.NetPacketConn)
	if !isReadWaiter {
		return nil, false
	}
	return &bindPacketReadWaiter{readWaiter}, true
}

func (c *bindPacketConn) RemoteAddr() net.Addr {
	return c.addr
}

func (c *bindPacketConn) Upstream() any {
	return c.NetPacketConn
}

var (
	_ N.NetPacketConn         = (*UnbindPacketConn)(nil)
	_ N.PacketReadWaitCreator = (*UnbindPacketConn)(nil)
)

type UnbindPacketConn struct {
	N.ExtendedConn
	addr M.Socksaddr
}

func NewUnbindPacketConn(conn net.Conn) N.NetPacketConn {
	return &UnbindPacketConn{
		NewExtendedConn(conn),
		M.SocksaddrFromNet(conn.RemoteAddr()),
	}
}

func NewUnbindPacketConnWithAddr(conn net.Conn, addr M.Socksaddr) N.NetPacketConn {
	return &UnbindPacketConn{
		NewExtendedConn(conn),
		addr,
	}
}

func (c *UnbindPacketConn) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	n, err = c.ExtendedConn.Read(p)
	if err == nil {
		addr = c.addr.UDPAddr()
	}
	return
}

func (c *UnbindPacketConn) WriteTo(p []byte, _ net.Addr) (n int, err error) {
	return c.ExtendedConn.Write(p)
}

func (c *UnbindPacketConn) ReadPacket(buffer *buf.Buffer) (destination M.Socksaddr, err error) {
	err = c.ExtendedConn.ReadBuffer(buffer)
	if err != nil {
		return
	}
	destination = c.addr
	return
}

func (c *UnbindPacketConn) WritePacket(buffer *buf.Buffer, _ M.Socksaddr) error {
	return c.ExtendedConn.WriteBuffer(buffer)
}

func (c *UnbindPacketConn) CreateReadWaiter() (N.PacketReadWaiter, bool) {
	readWaiter, isReadWaiter := CreateReadWaiter(c.ExtendedConn)
	if !isReadWaiter {
		return nil, false
	}
	return &unbindPacketReadWaiter{readWaiter, c.addr}, true
}

func (c *UnbindPacketConn) Upstream() any {
	return c.ExtendedConn
}

func NewServerPacketConn(conn net.PacketConn) N.ExtendedConn {
	return &serverPacketConn{
		NetPacketConn: NewPacketConn(conn),
	}
}

func NewServerPacketConnWithSource(conn net.PacketConn, expectedSource netip.Addr) N.ExtendedConn {
	return &serverPacketConn{
		NetPacketConn:  NewPacketConn(conn),
		expectedSource: expectedSource.Unmap(),
	}
}

type serverPacketConn struct {
	N.NetPacketConn
	expectedSource netip.Addr
	remoteAddr     atomic.Pointer[M.Socksaddr]
}

func (c *serverPacketConn) isAllowedSource(source M.Socksaddr) bool {
	if !c.expectedSource.IsValid() {
		return true
	}
	return source.Addr.Unmap() == c.expectedSource
}

func (c *serverPacketConn) Read(p []byte) (n int, err error) {
	for {
		n, addr, err := c.NetPacketConn.ReadFrom(p)
		if err != nil {
			return 0, err
		}
		source := M.SocksaddrFromNet(addr)
		if !c.isAllowedSource(source) {
			continue
		}
		c.remoteAddr.Store(&source)
		return n, nil
	}
}

func (c *serverPacketConn) ReadBuffer(buffer *buf.Buffer) error {
	for {
		destination, err := c.NetPacketConn.ReadPacket(buffer)
		if err != nil {
			return err
		}
		if !c.isAllowedSource(destination) {
			buffer.Truncate(0)
			continue
		}
		c.remoteAddr.Store(&destination)
		return nil
	}
}

func (c *serverPacketConn) Write(p []byte) (n int, err error) {
	remote := c.RemoteAddr().(M.Socksaddr)
	return c.NetPacketConn.WriteTo(p, remote.UDPAddr())
}

func (c *serverPacketConn) WriteBuffer(buffer *buf.Buffer) error {
	remote := c.RemoteAddr().(M.Socksaddr)
	return c.NetPacketConn.WritePacket(buffer, remote)
}

func (c *serverPacketConn) RemoteAddr() net.Addr {
	if addr := c.remoteAddr.Load(); addr != nil {
		return *addr
	}
	return M.Socksaddr{}
}

func (c *serverPacketConn) Upstream() any {
	return c.NetPacketConn
}

func (c *serverPacketConn) CreateReadWaiter() (N.ReadWaiter, bool) {
	readWaiter, isReadWaiter := CreatePacketReadWaiter(c.NetPacketConn)
	if !isReadWaiter {
		return nil, false
	}
	return &serverPacketReadWaiter{c, readWaiter}, true
}
