// Package osc implements the subset of Open Sound Control 1.0 that Sonic Pi
// speaks: messages and bundles carrying int32, int64, float32, float64,
// string, blob and boolean arguments, sent over UDP.
package osc

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"math"
	"net"
	"strings"
	"time"
)

// Message is a single OSC message.
type Message struct {
	Address string
	Args    []any
}

// NewMessage builds a message from an address and its arguments.
func NewMessage(address string, args ...any) *Message {
	return &Message{Address: address, Args: args}
}

func (m *Message) String() string {
	parts := make([]string, 0, len(m.Args))
	for _, a := range m.Args {
		parts = append(parts, fmt.Sprintf("%v", a))
	}
	return fmt.Sprintf("%s %s", m.Address, strings.Join(parts, " "))
}

// Int returns argument i as an int, converting numeric types.
func (m *Message) Int(i int) (int, bool) {
	if i >= len(m.Args) {
		return 0, false
	}
	switch v := m.Args[i].(type) {
	case int32:
		return int(v), true
	case int64:
		return int(v), true
	case float32:
		return int(v), true
	case float64:
		return int(v), true
	}
	return 0, false
}

// Str returns argument i formatted as a string.
func (m *Message) Str(i int) string {
	if i >= len(m.Args) {
		return ""
	}
	switch v := m.Args[i].(type) {
	case string:
		return v
	case []byte:
		return string(v)
	default:
		return fmt.Sprintf("%v", v)
	}
}

func pad4(n int) int { return (4 - n%4) % 4 }

func writeString(buf *bytes.Buffer, s string) {
	buf.WriteString(s)
	buf.WriteByte(0)
	for i := 0; i < pad4(len(s)+1); i++ {
		buf.WriteByte(0)
	}
}

// Encode serialises the message.
func (m *Message) Encode() ([]byte, error) {
	var buf bytes.Buffer
	writeString(&buf, m.Address)
	tags := []byte{','}
	var body bytes.Buffer
	for _, a := range m.Args {
		switch v := a.(type) {
		case int:
			if v >= math.MinInt32 && v <= math.MaxInt32 {
				tags = append(tags, 'i')
				binary.Write(&body, binary.BigEndian, int32(v))
			} else {
				tags = append(tags, 'h')
				binary.Write(&body, binary.BigEndian, int64(v))
			}
		case int32:
			tags = append(tags, 'i')
			binary.Write(&body, binary.BigEndian, v)
		case int64:
			tags = append(tags, 'h')
			binary.Write(&body, binary.BigEndian, v)
		case float32:
			tags = append(tags, 'f')
			binary.Write(&body, binary.BigEndian, v)
		case float64:
			tags = append(tags, 'd')
			binary.Write(&body, binary.BigEndian, v)
		case string:
			tags = append(tags, 's')
			writeString(&body, v)
		case []byte:
			tags = append(tags, 'b')
			binary.Write(&body, binary.BigEndian, int32(len(v)))
			body.Write(v)
			for i := 0; i < pad4(len(v)); i++ {
				body.WriteByte(0)
			}
		case bool:
			if v {
				tags = append(tags, 'T')
			} else {
				tags = append(tags, 'F')
			}
		case nil:
			tags = append(tags, 'N')
		default:
			return nil, fmt.Errorf("osc: unsupported argument type %T", a)
		}
	}
	writeString(&buf, string(tags))
	buf.Write(body.Bytes())
	return buf.Bytes(), nil
}

var errShort = errors.New("osc: packet too short")

func readString(b []byte, off int) (string, int, error) {
	end := bytes.IndexByte(b[off:], 0)
	if end < 0 {
		return "", 0, errShort
	}
	s := string(b[off : off+end])
	n := end + 1
	n += pad4(n)
	if off+n > len(b) {
		n = len(b) - off
	}
	return s, off + n, nil
}

// Decode parses a packet. Bundles are flattened into their messages.
func Decode(b []byte) ([]*Message, error) {
	if len(b) >= 8 && string(b[:8]) == "#bundle\x00" {
		return decodeBundle(b)
	}
	m, err := decodeMessage(b)
	if err != nil {
		return nil, err
	}
	return []*Message{m}, nil
}

func decodeBundle(b []byte) ([]*Message, error) {
	off := 16 // "#bundle\0" + 8-byte timetag
	var out []*Message
	for off+4 <= len(b) {
		size := int(binary.BigEndian.Uint32(b[off:]))
		off += 4
		if size < 0 || off+size > len(b) {
			return out, errShort
		}
		ms, err := Decode(b[off : off+size])
		if err != nil {
			return out, err
		}
		out = append(out, ms...)
		off += size
	}
	return out, nil
}

func decodeMessage(b []byte) (*Message, error) {
	addr, off, err := readString(b, 0)
	if err != nil {
		return nil, err
	}
	m := &Message{Address: addr}
	if off >= len(b) {
		return m, nil
	}
	tags, off, err := readString(b, off)
	if err != nil {
		return nil, err
	}
	if !strings.HasPrefix(tags, ",") {
		return m, nil
	}
	for _, t := range tags[1:] {
		switch t {
		case 'i':
			if off+4 > len(b) {
				return nil, errShort
			}
			m.Args = append(m.Args, int32(binary.BigEndian.Uint32(b[off:])))
			off += 4
		case 'h':
			if off+8 > len(b) {
				return nil, errShort
			}
			m.Args = append(m.Args, int64(binary.BigEndian.Uint64(b[off:])))
			off += 8
		case 'f':
			if off+4 > len(b) {
				return nil, errShort
			}
			m.Args = append(m.Args, math.Float32frombits(binary.BigEndian.Uint32(b[off:])))
			off += 4
		case 'd':
			if off+8 > len(b) {
				return nil, errShort
			}
			m.Args = append(m.Args, math.Float64frombits(binary.BigEndian.Uint64(b[off:])))
			off += 8
		case 's', 'S':
			var s string
			s, off, err = readString(b, off)
			if err != nil {
				return nil, err
			}
			m.Args = append(m.Args, s)
		case 'b':
			if off+4 > len(b) {
				return nil, errShort
			}
			n := int(binary.BigEndian.Uint32(b[off:]))
			off += 4
			if n < 0 || off+n > len(b) {
				return nil, errShort
			}
			m.Args = append(m.Args, append([]byte(nil), b[off:off+n]...))
			off += n + pad4(n)
		case 'T':
			m.Args = append(m.Args, true)
		case 'F':
			m.Args = append(m.Args, false)
		case 'N', 'I':
			m.Args = append(m.Args, nil)
		case 't':
			if off+8 > len(b) {
				return nil, errShort
			}
			m.Args = append(m.Args, int64(binary.BigEndian.Uint64(b[off:])))
			off += 8
		default:
			return nil, fmt.Errorf("osc: unsupported type tag %q", t)
		}
	}
	return m, nil
}

// Client sends messages to one UDP endpoint.
type Client struct {
	conn *net.UDPConn
}

// Dial opens a client to host:port.
func Dial(host string, port int) (*Client, error) {
	addr, err := net.ResolveUDPAddr("udp", net.JoinHostPort(host, fmt.Sprint(port)))
	if err != nil {
		return nil, err
	}
	conn, err := net.DialUDP("udp", nil, addr)
	if err != nil {
		return nil, err
	}
	return &Client{conn: conn}, nil
}

// Send encodes and sends a message.
func (c *Client) Send(address string, args ...any) error {
	b, err := NewMessage(address, args...).Encode()
	if err != nil {
		return err
	}
	_, err = c.conn.Write(b)
	return err
}

// Close closes the socket.
func (c *Client) Close() error { return c.conn.Close() }

// Server receives messages on a UDP port.
type Server struct {
	conn *net.UDPConn
}

// Listen binds 127.0.0.1:port. A port of 0 picks a free port.
func Listen(port int) (*Server, error) {
	conn, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port})
	if err != nil {
		return nil, err
	}
	conn.SetReadBuffer(4 << 20)
	return &Server{conn: conn}, nil
}

// Port reports the bound port.
func (s *Server) Port() int { return s.conn.LocalAddr().(*net.UDPAddr).Port }

// Serve reads packets until the socket is closed, calling fn for each
// decoded message.
func (s *Server) Serve(fn func(*Message)) error {
	buf := make([]byte, 65536)
	for {
		n, _, err := s.conn.ReadFromUDP(buf)
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				return nil
			}
			return err
		}
		msgs, err := Decode(buf[:n])
		if err != nil {
			continue
		}
		for _, m := range msgs {
			fn(m)
		}
	}
}

// SetDeadline sets a read deadline on the socket.
func (s *Server) SetDeadline(t time.Time) error { return s.conn.SetReadDeadline(t) }

// Close closes the socket.
func (s *Server) Close() error { return s.conn.Close() }
