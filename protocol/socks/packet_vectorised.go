package socks

import (
	"net"

	"github.com/metacubex/sing/common"
	"github.com/metacubex/sing/common/buf"
	"github.com/metacubex/sing/common/bufio"
	M "github.com/metacubex/sing/common/metadata"
	N "github.com/metacubex/sing/common/network"
)

var _ N.VectorisedPacketWriter = (*VectorisedAssociatePacketConn)(nil)

type VectorisedAssociatePacketConn struct {
	AssociatePacketConn
	N.VectorisedPacketWriter
}

func NewVectorisedAssociateConn(conn net.Conn, writer N.VectorisedWriter, remoteAddr M.Socksaddr, underlying net.Conn) *VectorisedAssociatePacketConn {
	c := &VectorisedAssociatePacketConn{
		AssociatePacketConn: AssociatePacketConn{
			AbstractConn: conn,
			conn:         bufio.NewExtendedConn(conn),
			underlying:   underlying,
		},
		VectorisedPacketWriter: &bufio.UnbindVectorisedPacketWriter{VectorisedWriter: writer},
	}
	c.remoteAddr.Store(&remoteAddr)
	return c
}

func (c *VectorisedAssociatePacketConn) WriteVectorisedPacket(buffers []*buf.Buffer, destination M.Socksaddr) error {
	header := buf.NewSize(3 + M.SocksaddrSerializer.AddrPortLen(destination))
	defer header.Release()
	common.Must(header.WriteZeroN(3))
	err := M.SocksaddrSerializer.WriteAddrPort(header, destination)
	if err != nil {
		return err
	}
	return c.VectorisedPacketWriter.WriteVectorisedPacket(append([]*buf.Buffer{header}, buffers...), destination)
}

func (c *VectorisedAssociatePacketConn) FrontHeadroom() int {
	return 0
}
