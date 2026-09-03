// Package control is the board-client unix-socket JSON RPC used by wrap and config UI.
package control

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net"
	"os"
	"strings"
	"time"
)

const maxLine = 1024 * 1024

// Request is one control message.
type Request struct {
	Op         string `json:"op"`
	RunKey     string `json:"run_key,omitempty"`
	PID        int    `json:"pid,omitempty"`
	Summary    string `json:"summary,omitempty"`
	TTLSeconds int    `json:"ttl_seconds,omitempty"`
	LogPath    string `json:"log_path,omitempty"`
	Chunk      string `json:"chunk,omitempty"`
	ExitCode   *int   `json:"exit_code,omitempty"`
}

// Response is the RPC result.
type Response struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

// Handler processes one request.
type Handler func(Request) Response

// Server accepts newline-delimited JSON on a unix socket.
type Server struct {
	Path   string
	Handle Handler
	ln     net.Listener
}

// Listen binds Path, replacing a stale socket.
func Listen(path string, h Handler) (*Server, error) {
	if path == "" {
		return nil, fmt.Errorf("control socket path is empty")
	}
	_ = os.Remove(path)
	ln, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		_ = ln.Close()
		_ = os.Remove(path)
		return nil, err
	}
	return &Server{Path: path, Handle: h, ln: ln}, nil
}

// Serve runs until ctx is cancelled.
func (s *Server) Serve(ctx context.Context) error {
	go func() {
		<-ctx.Done()
		_ = s.ln.Close()
	}()
	for {
		conn, err := s.ln.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		go s.serveConn(conn)
	}
}

func (s *Server) serveConn(conn net.Conn) {
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(30 * time.Second))
	sc := bufio.NewScanner(conn)
	buf := make([]byte, 64*1024)
	sc.Buffer(buf, maxLine)
	for sc.Scan() {
		_ = conn.SetDeadline(time.Now().Add(30 * time.Second))
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var req Request
		if err := json.Unmarshal([]byte(line), &req); err != nil {
			writeResp(conn, Response{Error: "invalid json"})
			return
		}
		resp := Response{OK: true}
		if s.Handle != nil {
			resp = s.Handle(req)
		}
		writeResp(conn, resp)
		if req.Op == "wrap_exit" {
			return
		}
	}
}

func writeResp(conn net.Conn, resp Response) {
	b, _ := json.Marshal(resp)
	b = append(b, '\n')
	_, _ = conn.Write(b)
}

// Call sends one request and reads the response.
func Call(path string, req Request, timeout time.Duration) (Response, error) {
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	conn, err := net.DialTimeout("unix", path, timeout)
	if err != nil {
		return Response{}, err
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(timeout))
	b, err := json.Marshal(req)
	if err != nil {
		return Response{}, err
	}
	if _, err := conn.Write(append(b, '\n')); err != nil {
		return Response{}, err
	}
	sc := bufio.NewScanner(conn)
	buf := make([]byte, 4096)
	sc.Buffer(buf, 64*1024)
	if !sc.Scan() {
		if err := sc.Err(); err != nil {
			return Response{}, err
		}
		return Response{}, fmt.Errorf("no control response")
	}
	var resp Response
	if err := json.Unmarshal(sc.Bytes(), &resp); err != nil {
		return Response{}, err
	}
	return resp, nil
}

// Session is a persistent connection for wrap_start / stdout / exit.
type Session struct {
	conn net.Conn
	rd   *bufio.Reader
}

// Dial opens a control session.
func Dial(path string, timeout time.Duration) (*Session, error) {
	if timeout <= 0 {
		timeout = 3 * time.Second
	}
	conn, err := net.DialTimeout("unix", path, timeout)
	if err != nil {
		return nil, err
	}
	return &Session{conn: conn, rd: bufio.NewReaderSize(conn, 64*1024)}, nil
}

// Do sends req and waits for a response.
func (s *Session) Do(req Request, timeout time.Duration) (Response, error) {
	if s == nil || s.conn == nil {
		return Response{}, fmt.Errorf("control not connected")
	}
	if timeout <= 0 {
		timeout = 5 * time.Second
	}
	_ = s.conn.SetDeadline(time.Now().Add(timeout))
	b, err := json.Marshal(req)
	if err != nil {
		return Response{}, err
	}
	if _, err := s.conn.Write(append(b, '\n')); err != nil {
		return Response{}, err
	}
	line, err := s.rd.ReadBytes('\n')
	if err != nil {
		return Response{}, err
	}
	var resp Response
	if err := json.Unmarshal(line, &resp); err != nil {
		return Response{}, err
	}
	return resp, nil
}

// Close the session.
func (s *Session) Close() error {
	if s == nil || s.conn == nil {
		return nil
	}
	return s.conn.Close()
}
