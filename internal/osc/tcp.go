package osc

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"sync"
	"time"
)

// TCPClient sends messages as length-prefixed frames (a 4-byte big-endian
// size, then the packet) over one TCP connection, which is how Sonic Pi 5's
// GUI talks to the SuperSonic engine. It connects on first use and again
// after a failure, and discards whatever the engine writes back.
type TCPClient struct {
	addr string
	mu   sync.Mutex
	conn net.Conn
}

// NewTCPClient makes a client for host:port without connecting yet.
func NewTCPClient(host string, port int) *TCPClient {
	return &TCPClient{addr: net.JoinHostPort(host, fmt.Sprint(port))}
}

// Frame encodes a message as one length-prefixed frame.
func Frame(m *Message) ([]byte, error) {
	b, err := m.Encode()
	if err != nil {
		return nil, err
	}
	out := make([]byte, 4+len(b))
	binary.BigEndian.PutUint32(out, uint32(len(b)))
	copy(out[4:], b)
	return out, nil
}

// Send encodes and sends a message, reconnecting once if the connection
// has dropped.
func (c *TCPClient) Send(address string, args ...any) error {
	frame, err := Frame(NewMessage(address, args...))
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	for attempt := 0; attempt < 2; attempt++ {
		if c.conn == nil {
			conn, err := net.DialTimeout("tcp", c.addr, 2*time.Second)
			if err != nil {
				return err
			}
			if tc, ok := conn.(*net.TCPConn); ok {
				tc.SetNoDelay(true)
			}
			c.conn = conn
			go io.Copy(io.Discard, conn)
		}
		if _, err = c.conn.Write(frame); err == nil {
			return nil
		}
		c.conn.Close()
		c.conn = nil
	}
	return err
}

// Close closes the connection.
func (c *TCPClient) Close() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.conn == nil {
		return nil
	}
	err := c.conn.Close()
	c.conn = nil
	return err
}
