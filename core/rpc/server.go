package rpc

import (
	"bufio"
	"encoding/json"
	"errors"
	"net"
	"os"
	"sync"
	"sync/atomic"
	"time"

	"git.arianw.de/shrippen/hansei/core/service"
)

const (
	maxMessage = 64 << 20
	socketMode = 0o600
)

// Server serves one API on a Unix socket.
type Server struct {
	api     service.API
	ln      net.Listener
	clients atomic.Int64
	last    atomic.Int64 // unix time of the last client activity
	wg      sync.WaitGroup
}

// Listen creates the socket (removing a stale one) with owner-only permissions.
func Listen(api service.API, path string) (*Server, error) {
	if c, err := net.Dial("unix", path); err == nil {
		c.Close()
		return nil, errors.New("rpc: a daemon is already listening on " + path)
	}
	_ = os.Remove(path)
	old := umask(0o177)
	ln, err := net.Listen("unix", path)
	umask(old)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, socketMode); err != nil {
		ln.Close()
		return nil, err
	}
	s := &Server{api: api, ln: ln}
	s.last.Store(time.Now().Unix())
	return s, nil
}

// Serve accepts clients until Close.
func (s *Server) Serve() error {
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			if errors.Is(err, net.ErrClosed) {
				s.wg.Wait()
				return nil
			}
			return err
		}
		s.wg.Add(1)
		go s.handle(conn)
	}
}

// Close stops accepting and removes the socket.
func (s *Server) Close() error {
	addr := s.ln.Addr().String()
	err := s.ln.Close()
	_ = os.Remove(addr)
	return err
}

// Idle reports how long no client was connected.
func (s *Server) Idle() time.Duration {
	if s.clients.Load() > 0 {
		return 0
	}
	return time.Since(time.Unix(s.last.Load(), 0))
}

// conn is one client with a write lock shared by responses and events.
type conn struct {
	c  net.Conn
	mu sync.Mutex
	w  *bufio.Writer
}

func (c *conn) send(v any) error {
	raw, err := json.Marshal(v)
	if err != nil {
		return err
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	if _, err := c.w.Write(append(raw, '\n')); err != nil {
		return err
	}
	return c.w.Flush()
}

func (s *Server) handle(nc net.Conn) {
	defer s.wg.Done()
	defer nc.Close()
	s.clients.Add(1)
	defer func() {
		s.clients.Add(-1)
		s.last.Store(time.Now().Unix())
	}()

	c := &conn{c: nc, w: bufio.NewWriter(nc)}
	var stop func()
	defer func() {
		if stop != nil {
			stop()
		}
	}()

	sc := bufio.NewScanner(nc)
	sc.Buffer(make([]byte, 0, 1<<20), maxMessage)
	for sc.Scan() {
		var req request
		if err := json.Unmarshal(sc.Bytes(), &req); err != nil {
			_ = c.send(response{JSONRPC: "2.0", Error: &rpcError{Code: codeParse, Message: err.Error()}})
			continue
		}
		if req.Method == "subscribe" {
			if stop == nil {
				stop = s.stream(c)
			}
			_ = c.send(response{JSONRPC: "2.0", ID: req.ID, Result: ok{true}})
			continue
		}
		// Calls run concurrently so a slow one (e.g. findings) does not block the event stream.
		go func(req request) {
			res := s.call(req)
			if len(req.ID) > 0 {
				_ = c.send(res)
			}
		}(req)
	}
}

// stream forwards service events as notifications.
func (s *Server) stream(c *conn) func() {
	events, stop := s.api.Subscribe()
	go func() {
		for e := range events {
			if c.send(response{JSONRPC: "2.0", Method: "event", Params: e}) != nil {
				return
			}
		}
	}()
	return stop
}

func (s *Server) call(req request) (res response) {
	res = response{JSONRPC: "2.0", ID: req.ID}
	if req.JSONRPC != "2.0" || req.Method == "" {
		res.Error = &rpcError{Code: codeInvalid, Message: "invalid request"}
		return res
	}
	h, ok := Methods[req.Method]
	if !ok {
		res.Error = &rpcError{Code: codeMethod, Message: "unknown method " + req.Method}
		return res
	}
	defer func() {
		if r := recover(); r != nil {
			res.Result, res.Error = nil, &rpcError{Code: codeInternal, Message: "internal error"}
		}
	}()
	out, err := h(s.api, req.Params)
	if err != nil {
		var re *rpcError
		if errors.As(err, &re) {
			res.Error = re
			return res
		}
		res.Error = &rpcError{Code: codeInternal, Message: err.Error()}
		return res
	}
	res.Result = out
	return res
}
