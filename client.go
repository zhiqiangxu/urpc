package urpc

import "net"

// Connection for urpc
type Connection struct {
	*net.UDPConn
}

// NewConnection is ctor for Connection
func NewConnection(addr string) (conn *Connection, err error) {

	udpConn, err := net.Dial("udp", addr)
	if err != nil {
		return
	}

	conn = &Connection{udpConn}
	return
}
