package socks

import (
	"bytes"
	"net"
	"sync/atomic"

	"github.com/metacubex/sing/common"
	"github.com/metacubex/sing/common/buf"
	"github.com/metacubex/sing/common/bufio"
	E "github.com/metacubex/sing/common/exceptions"
	M "github.com/metacubex/sing/common/metadata"
	N "github.com/metacubex/sing/common/network"
)

// +----+------+------+----------+----------+----------+
// |RSV | FRAG | ATYP | DST.ADDR | DST.PORT |   DATA   |
// +----+------+------+----------+----------+----------+
// | 2  |  1   |  1   | Variable |    2     | Variable |
// +----+------+------+----------+----------+----------+

var ErrInvalidPacket = E.New("socks5: invalid packet")

type AssociatePacketConn struct {
	N.AbstractConn
	conn       N.ExtendedConn
	remoteAddr atomic.Pointer[M.Socksaddr]
	underlying net.Conn
}

func NewAssociatePacketConn(conn net.Conn, remoteAddr M.Socksaddr, underlying net.Conn) *AssociatePacketConn {
	c := &AssociatePacketConn{
		AbstractConn: conn,
		conn:         bufio.NewExtendedConn(conn),
		underlying:   underlying,
	}
	c.remoteAddr.Store(&remoteAddr)
	return c
}

func (c *AssociatePacketConn) ReadFrom(p []byte) (n int, addr net.Addr, err error) {
	n, err = c.conn.Read(p)
	if err != nil {
		return
	}
	if n < 3 || p[2] != 0 {
		return 0, nil, ErrInvalidPacket
	}
	reader := bytes.NewReader(p[3:n])
	destination, err := M.SocksaddrSerializer.ReadAddrPort(reader)
	if err != nil {
		return
	}
	c.remoteAddr.Store(&destination)
	addr = destination.UDPAddr()
	index := 3 + int(reader.Size()) - reader.Len()
	n = copy(p, p[index:n])
	return
}

func (c *AssociatePacketConn) WriteTo(p []byte, addr net.Addr) (n int, err error) {
	destination := M.SocksaddrFromNet(addr)
	buffer := buf.NewSize(3 + M.SocksaddrSerializer.AddrPortLen(destination) + len(p))
	defer buffer.Release()
	common.Must(buffer.WriteZeroN(3))
	err = M.SocksaddrSerializer.WriteAddrPort(buffer, destination)
	if err != nil {
		return
	}
	_, err = buffer.Write(p)
	if err != nil {
		return
	}
	return c.conn.Write(buffer.Bytes())
}

func (c *AssociatePacketConn) ReadPacket(buffer *buf.Buffer) (destination M.Socksaddr, err error) {
	err = c.conn.ReadBuffer(buffer)
	if err != nil {
		return
	}
	if buffer.Len() < 3 || buffer.Byte(2) != 0 {
		return M.Socksaddr{}, ErrInvalidPacket
	}
	buffer.Advance(3)
	destination, err = M.SocksaddrSerializer.ReadAddrPort(buffer)
	if err != nil {
		return
	}
	c.remoteAddr.Store(&destination)
	return destination.Unwrap(), nil
}

func (c *AssociatePacketConn) WritePacket(buffer *buf.Buffer, destination M.Socksaddr) error {
	header := buf.With(buffer.ExtendHeader(3 + M.SocksaddrSerializer.AddrPortLen(destination)))
	common.Must(header.WriteZeroN(3))
	err := M.SocksaddrSerializer.WriteAddrPort(header, destination)
	if err != nil {
		return err
	}
	return c.conn.WriteBuffer(buffer)
}

func (c *AssociatePacketConn) Read(b []byte) (n int, err error) {
	n, _, err = c.ReadFrom(b)
	return
}

func (c *AssociatePacketConn) Write(b []byte) (n int, err error) {
	remote := c.remoteAddr.Load()
	if remote == nil {
		return 0, ErrInvalidPacket
	}
	return c.WriteTo(b, *remote)
}

func (c *AssociatePacketConn) RemoteAddr() net.Addr {
	if remote := c.remoteAddr.Load(); remote != nil {
		return remote.UDPAddr()
	}
	return M.Socksaddr{}.UDPAddr()
}

func (c *AssociatePacketConn) Upstream() any {
	return c.conn
}

func (c *AssociatePacketConn) FrontHeadroom() int {
	return 3 + M.MaxSocksaddrLength
}

func (c *AssociatePacketConn) Close() error {
	return common.Close(
		c.conn,
		c.underlying,
	)
}
