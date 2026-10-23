package service

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/AlexDev404/sonic-pi-lsp/internal/control"
	"github.com/AlexDev404/sonic-pi-lsp/internal/events"
	"github.com/AlexDev404/sonic-pi-lsp/internal/prefs"
)

// Server answers the CLI on the control socket.
type Server struct {
	Sup      *Supervisor
	Token    string
	Shutdown func()
	ln       net.Listener
}

// Listen binds the control socket on a free localhost port.
func (srv *Server) Listen() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	srv.ln = ln
	return ln.Addr().(*net.TCPAddr).Port, nil
}

// Serve accepts connections until ctx ends.
func (srv *Server) Serve(ctx context.Context) {
	go func() {
		<-ctx.Done()
		srv.ln.Close()
	}()
	for {
		c, err := srv.ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return
			}
			continue
		}
		go srv.handle(ctx, c)
	}
}

type conn struct {
	net.Conn
	w *bufio.Writer
}

func (c *conn) send(r control.Response) error {
	b, _ := json.Marshal(r)
	c.w.Write(append(b, '\n'))
	return c.w.Flush()
}

func (c *conn) done(data any, err error) {
	r := control.Response{Done: true}
	if err != nil {
		r.Error = err.Error()
	} else if data != nil {
		r.Data, _ = json.Marshal(data)
	}
	c.send(r)
}

// hangup returns a channel closed when the client goes away.
func (c *conn) hangup(r *bufio.Reader) <-chan struct{} {
	ch := make(chan struct{})
	go func() {
		buf := make([]byte, 1)
		for {
			if _, err := r.Read(buf); err != nil {
				close(ch)
				return
			}
		}
	}()
	return ch
}

func (srv *Server) handle(ctx context.Context, nc net.Conn) {
	defer nc.Close()
	c := &conn{Conn: nc, w: bufio.NewWriter(nc)}
	r := bufio.NewReaderSize(nc, 1<<20)
	nc.SetReadDeadline(time.Now().Add(10 * time.Second))
	line, err := r.ReadBytes('\n')
	if err != nil {
		return
	}
	nc.SetReadDeadline(time.Time{})
	var req control.Request
	if err := json.Unmarshal(line, &req); err != nil {
		c.done(nil, errors.New("bad request"))
		return
	}
	if req.Token != srv.Token {
		c.done(nil, errors.New("bad token"))
		return
	}
	sup := srv.Sup
	switch req.Cmd {
	case "status":
		var a control.WaitArgs
		json.Unmarshal(req.Args, &a)
		if !a.Wait {
			c.done(sup.Status(), nil)
			return
		}
		wctx, cancel := context.WithTimeout(ctx, seconds(a.Timeout, BootTimeout))
		defer cancel()
		go func() {
			<-c.hangup(r)
			cancel()
		}()
		c.done(sup.WaitBooted(wctx), nil)

	case "run":
		var a control.RunArgs
		if err := json.Unmarshal(req.Args, &a); err != nil {
			c.done(nil, err)
			return
		}
		if err := srv.waitRunning(ctx); err != nil {
			c.done(nil, err)
			return
		}
		c.done(nil, sup.RunCode(a.Code))

	case "subscribe":
		ch, unsub := sup.Subscribe()
		defer unsub()
		c.send(control.Response{Event: events.Service("subscribed")})
		gone := c.hangup(r)
		for {
			select {
			case e := <-ch:
				if c.send(control.Response{Event: e}) != nil {
					return
				}
			case <-gone:
				return
			case <-ctx.Done():
				c.done(nil, nil)
				return
			}
		}

	case "send":
		var a control.SendArgs
		if err := json.Unmarshal(req.Args, &a); err != nil {
			c.done(nil, err)
			return
		}
		if err := srv.waitRunning(ctx); err != nil {
			c.done(nil, err)
			return
		}
		args := make([]any, len(a.Args))
		for i, v := range a.Args {
			args[i] = v.Value()
		}
		if a.Expect == "" {
			c.done(nil, sup.Send(a.Target, a.Address, args...))
			return
		}
		ch, unsub := sup.Subscribe()
		defer unsub()
		if err := sup.Send(a.Target, a.Address, args...); err != nil {
			c.done(nil, err)
			return
		}
		timeout := time.After(seconds(a.Timeout, 10*time.Second))
		for {
			select {
			case e := <-ch:
				if e.Kind == a.Expect {
					c.done(e, nil)
					return
				}
			case <-timeout:
				c.done(nil, fmt.Errorf("no answer from the server to %s", a.Address))
				return
			case <-ctx.Done():
				c.done(nil, errors.New("the service is shutting down"))
				return
			}
		}

	case "apply-prefs":
		st, err := prefs.Load(sup.Root)
		if err != nil {
			c.done(nil, err)
			return
		}
		sup.ApplyPrefs(st, false)
		c.done(nil, nil)

	case "restart":
		sup.Restart()
		c.done(nil, nil)

	case "shutdown":
		c.done(nil, nil)
		if srv.Shutdown != nil {
			srv.Shutdown()
		}

	default:
		c.done(nil, fmt.Errorf("unknown command %q", req.Cmd))
	}
}

// waitRunning waits for the server to boot before a command that needs it.
func (srv *Server) waitRunning(ctx context.Context) error {
	wctx, cancel := context.WithTimeout(ctx, BootTimeout)
	defer cancel()
	st := srv.Sup.WaitBooted(wctx)
	switch st.State {
	case control.StateRunning:
		return nil
	case control.StateFailed:
		return fmt.Errorf("the Sonic Pi server failed to boot: %s", st.Error)
	}
	return ErrNotBooted
}

func seconds(s float64, def time.Duration) time.Duration {
	if s <= 0 {
		return def
	}
	return time.Duration(s * float64(time.Second))
}
