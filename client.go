package urpc

import (
	"errors"
	"net"
	"time"
)

// Connection for urpc
type Connection struct {
	udpConn *net.UDPConn
	conf    ConnectionConfig
}

// ConnectionConfig is config for Connection
type ConnectionConfig struct {
	DefaultReadTimeout  int
	DefaultWriteTimeout int
}

var (
	errNotUDPConn = errors.New("not net.UDPConn")
)

// NewConnection is ctor for Connection
func NewConnection(addr string, conf ConnectionConfig) (conn *Connection, err error) {

	c, err := net.Dial("udp", addr)
	if err != nil {
		return
	}

	udpConn, ok := c.(*net.UDPConn)
	if !ok {
		err = errNotUDPConn
		return
	}
	conn = &Connection{udpConn: udpConn, conf: conf}
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

	if conn.conf.DefaultWriteTimeout > 0 {
		endTime := time.Now().Add(time.Duration(conn.conf.DefaultWriteTimeout) * time.Second)
		err = conn.udpConn.SetWriteDeadline(endTime)
		if err != nil {
			return
		}
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

	if conn.conf.DefaultReadTimeout > 0 {
		endTime := time.Now().Add(time.Duration(conn.conf.DefaultReadTimeout) * time.Second)
		err = conn.udpConn.SetReadDeadline(endTime)
		if err != nil {
			return
		}
	}

	nbytes, err := conn.udpConn.Read(bytes)
	if err != nil {
		return
	}

	packet, err = decodePacket(bytes[0:nbytes])
	return
}
