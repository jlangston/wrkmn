package control

import (
	"bufio"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"time"
)

// Handler answers one validated request. The TUI's handler forwards the
// request into the Bubble Tea program and blocks until the model replies.
type Handler func(Request) Response

// requestReadTimeout bounds how long a connected client may take to send its
// request line; the reply itself is bounded by the handler.
const requestReadTimeout = 10 * time.Second

// Server serves the control protocol on a Unix socket.
type Server struct {
	path    string
	handler Handler

	mu sync.Mutex
	ln net.Listener
	wg sync.WaitGroup
}

// NewServer prepares a server; nothing listens until Start.
func NewServer(path string, handler Handler) *Server {
	return &Server{path: path, handler: handler}
}

// Path is the socket path the server listens on.
func (s *Server) Path() string { return s.path }

// Start creates the socket directory (0700), clears a stale socket file,
// listens, and serves in the background. A socket that still answers belongs
// to a live wrkmn for the same project and is never clobbered.
func (s *Server) Start() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.ln != nil {
		return errors.New("control server already started")
	}
	if err := os.MkdirAll(filepath.Dir(s.path), 0o700); err != nil {
		return fmt.Errorf("creating socket dir: %w", err)
	}
	if _, err := os.Lstat(s.path); err == nil {
		if conn, err := net.DialTimeout("unix", s.path, 500*time.Millisecond); err == nil {
			conn.Close()
			return fmt.Errorf("another wrkmn is already serving %s", s.path)
		}
		if err := os.Remove(s.path); err != nil {
			return fmt.Errorf("removing stale socket: %w", err)
		}
	}
	ln, err := net.Listen("unix", s.path)
	if err != nil {
		return fmt.Errorf("listening on %s: %w", s.path, err)
	}
	if err := os.Chmod(s.path, 0o600); err != nil {
		ln.Close()
		return fmt.Errorf("securing socket: %w", err)
	}
	s.ln = ln
	s.wg.Add(1)
	go s.serve(ln)
	return nil
}

func (s *Server) serve(ln net.Listener) {
	defer s.wg.Done()
	for {
		conn, err := ln.Accept()
		if err != nil {
			return // listener closed
		}
		s.wg.Add(1)
		go func() {
			defer s.wg.Done()
			s.handleConn(conn)
		}()
	}
}

func (s *Server) handleConn(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(requestReadTimeout))
	req, err := ReadRequest(bufio.NewReader(conn))
	if err != nil {
		_ = WriteResponse(conn, Errorf("bad request: %v", err))
		return
	}
	if err := req.Validate(); err != nil {
		_ = WriteResponse(conn, Errorf("bad request: %v", err))
		return
	}
	_ = conn.SetReadDeadline(time.Time{})
	_ = WriteResponse(conn, s.handler(req))
}

// Close stops listening, waits for in-flight connections, and removes the
// socket file. Safe to call more than once.
func (s *Server) Close() error {
	s.mu.Lock()
	ln := s.ln
	s.ln = nil
	s.mu.Unlock()
	if ln == nil {
		return nil
	}
	err := ln.Close()
	s.wg.Wait()
	if rmErr := os.Remove(s.path); rmErr != nil && !os.IsNotExist(rmErr) && err == nil {
		err = rmErr
	}
	return err
}
