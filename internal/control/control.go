// Package control is how the CLI talks to the service: newline-delimited
// JSON over a localhost TCP socket, authenticated by a token the service
// publishes in its state file.
package control

import (
	"bufio"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/AlexDev404/sonic-pi-lsp/internal/events"
	"github.com/AlexDev404/sonic-pi-lsp/internal/paths"
)

// StateFileName is the file a running service publishes in its state dir.
const StateFileName = "sonicpid.json"

// State is what a running service publishes about itself.
type State struct {
	PID         int       `json:"pid"`
	ControlPort int       `json:"control_port"`
	Token       string    `json:"token"`
	LogDir      string    `json:"log_dir"`
	Root        string    `json:"root"`
	Mode        string    `json:"mode"` // "service" (system service manager) or "user" (started on demand)
	Version     string    `json:"version"`
	Started     time.Time `json:"started"`
	path        string
}

// Path is the state file this was read from.
func (s *State) Path() string { return s.path }

// NewToken makes a random token.
func NewToken() string {
	b := make([]byte, 24)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// WriteState publishes s in dir, readable only by its owner where the OS
// allows it.
func WriteState(dir string, s *State) error {
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	b, _ := json.MarshalIndent(s, "", "  ")
	p := filepath.Join(dir, StateFileName)
	tmp := p + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	s.path = p
	return os.Rename(tmp, p)
}

// RemoveState deletes the state file if it still belongs to pid.
func RemoveState(dir string, pid int) {
	p := filepath.Join(dir, StateFileName)
	if s, err := readState(p); err == nil && s.PID == pid {
		os.Remove(p)
	}
}

func readState(p string) (*State, error) {
	b, err := os.ReadFile(p)
	if err != nil {
		return nil, err
	}
	var s State
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, err
	}
	s.path = p
	return &s, nil
}

// ErrNotRunning means no service answered.
var ErrNotRunning = errors.New("the Sonic Pi service is not running")

// Find returns the state of a service that answers on its control port.
func Find() (*State, error) {
	for _, d := range paths.StateDirs() {
		s, err := readState(filepath.Join(d, StateFileName))
		if err != nil {
			continue
		}
		c, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", s.ControlPort), 500*time.Millisecond)
		if err != nil {
			continue
		}
		c.Close()
		return s, nil
	}
	return nil, ErrNotRunning
}

// Request is one command sent to the service.
type Request struct {
	Token string          `json:"token"`
	Cmd   string          `json:"cmd"`
	Args  json.RawMessage `json:"args,omitempty"`
}

// Response is the service's answer. A streaming command sends any number of
// responses carrying Event before the final one, which has Done set.
type Response struct {
	Done  bool            `json:"done,omitempty"`
	Error string          `json:"error,omitempty"`
	Data  json.RawMessage `json:"data,omitempty"`
	Event *events.Event   `json:"event,omitempty"`
}

// Client is a connection to the service.
type Client struct {
	State *State
}

// Dial connects to the running service.
func Dial() (*Client, error) {
	s, err := Find()
	if err != nil {
		return nil, err
	}
	return &Client{State: s}, nil
}

func (c *Client) open(cmd string, args any) (net.Conn, *bufio.Reader, error) {
	conn, err := net.DialTimeout("tcp", fmt.Sprintf("127.0.0.1:%d", c.State.ControlPort), 2*time.Second)
	if err != nil {
		return nil, nil, ErrNotRunning
	}
	var raw json.RawMessage
	if args != nil {
		raw, _ = json.Marshal(args)
	}
	b, _ := json.Marshal(Request{Token: c.State.Token, Cmd: cmd, Args: raw})
	if _, err := conn.Write(append(b, '\n')); err != nil {
		conn.Close()
		return nil, nil, err
	}
	r := bufio.NewReaderSize(conn, 1<<20)
	return conn, r, nil
}

func readResponse(r *bufio.Reader) (*Response, error) {
	line, err := r.ReadBytes('\n')
	if err != nil {
		return nil, err
	}
	var resp Response
	if err := json.Unmarshal(line, &resp); err != nil {
		return nil, err
	}
	return &resp, nil
}

// Call sends a command and decodes its result into out (which may be nil).
func (c *Client) Call(cmd string, args any, out any) error {
	return c.Stream(cmd, args, out, nil)
}

// Stream sends a command, calling fn for each event until the final
// response, whose data is decoded into out. Closing stop ends the stream
// early; Stream then returns nil.
func (c *Client) Stream(cmd string, args any, out any, fn func(*events.Event), stop ...<-chan struct{}) error {
	conn, r, err := c.open(cmd, args)
	if err != nil {
		return err
	}
	defer conn.Close()
	if len(stop) > 0 && stop[0] != nil {
		done := make(chan struct{})
		defer close(done)
		go func() {
			select {
			case <-stop[0]:
				conn.Close()
			case <-done:
			}
		}()
	}
	for {
		resp, err := readResponse(r)
		if err != nil {
			if len(stop) > 0 && stop[0] != nil {
				select {
				case <-stop[0]:
					return nil
				default:
				}
			}
			return fmt.Errorf("lost the connection to the service: %w", err)
		}
		if resp.Event != nil && fn != nil {
			fn(resp.Event)
		}
		if !resp.Done {
			continue
		}
		if resp.Error != "" {
			return errors.New(resp.Error)
		}
		if out != nil && len(resp.Data) > 0 {
			return json.Unmarshal(resp.Data, out)
		}
		return nil
	}
}
