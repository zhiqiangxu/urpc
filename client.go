package urpc

import (
	"errors"
	"net"
)

// Connection for urpc
type Connection struct {
	udpConn *net.UDPConn
}

var (
	errNotUDPConn = errors.New("not net.UDPConn")
)

// NewConnection is ctor for Connection
func NewConnection(addr string) (conn *Connection, err error) {

	c, err := net.Dial("udp", addr)
	if err != nil {
		return
	}

	udpConn, ok := c.(*net.UDPConn)
	if !ok {
		err = errNotUDPConn
		return
	}
	conn = &Connection{udpConn: udpConn}
	return
}

var (
	errShortWrite = errors.New("short write")
)

// WritePacket for write urpc packet
func (conn *Connection) WritePacket(cmd Cmd, payload []byte) (err error) {

	bytes, err := encodePacket(cmd, payload)
	if err != nil {
		return
	}
	nbytes, err := conn.udpConn.Write(bytes)
	if err != nil {
		return
	}
	if nbytes != len(bytes) {
		err = errShortWrite
	}

	return
}

// ReadPacket for read urpc packet
func (conn *Connection) ReadPacket(bytes []byte) (packet Packet, err error) {
	nbytes, err := conn.udpConn.Read(bytes)
	if err != nil {
		return
	}

	packet, err = decodePacket(bytes[0:nbytes])
	return
}
