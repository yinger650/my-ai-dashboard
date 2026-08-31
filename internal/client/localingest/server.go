// Package localingest accepts a loopback copy of agent events. board-client
// projects them onto proj-* services with its own Machine Token and tees
// log.append for on-host AI digest. report.py still posts the original
// identity to board-server with the skill token.
package localingest

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"time"

	"agentboard/internal/event"
	"agentboard/internal/shared"
)

const maxBody = 512 * 1024

// Server is a loopback-only tee endpoint for Cursor/Codex log.append copies.
type Server struct {
	log         *slog.Logger
	listen      string
	http        *http.Server
	OnLogAppend func(serviceKey, markdown, severity, occurredAt, source string)
	OnEvent     func(env event.Envelope)
}

// New constructs a local ingest HTTP server. listen must be 127.0.0.1 or localhost.
func New(log *slog.Logger, listen string) (*Server, error) {
	if listen == "" {
		listen = "127.0.0.1:7438"
	}
	if err := checkLoopback(listen); err != nil {
		return nil, err
	}
	s := &Server{log: log, listen: listen}
	mux := http.NewServeMux()
	mux.HandleFunc("/health", s.handleHealth)
	mux.HandleFunc("/ingest/v1/events", s.handleEvents)
	s.http = &http.Server{
		Addr:              listen,
		Handler:           mux,
		ReadHeaderTimeout: 4 * time.Second,
		ReadTimeout:       8 * time.Second,
		WriteTimeout:      8 * time.Second,
		BaseContext:       func(net.Listener) context.Context { return context.Background() },
	}
	return s, nil
}

// Handler exposes the HTTP handler for tests.
func (s *Server) Handler() http.Handler { return s.http.Handler }

func checkLoopback(listen string) error {
	host, _, err := net.SplitHostPort(listen)
	if err != nil {
		return fmt.Errorf("local_ingest.listen: %w", err)
	}
	ip := net.ParseIP(host)
	if host == "localhost" || (ip != nil && ip.IsLoopback()) {
		return nil
	}
	return fmt.Errorf("local_ingest.listen must be loopback, got %q", listen)
}

func (s *Server) handleHealth(w http.ResponseWriter, _ *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(`{"ok":true,"service":"board-client-local-ingest"}`))
}

func (s *Server) handleEvents(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	r.Body = http.MaxBytesReader(w, r.Body, maxBody)
	raw, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "payload too large", http.StatusRequestEntityTooLarge)
		return
	}
	var batch event.Batch
	if err := json.Unmarshal(raw, &batch); err != nil {
		http.Error(w, "invalid json", http.StatusBadRequest)
		return
	}
	if len(batch.Events) == 0 {
		http.Error(w, "events required", http.StatusBadRequest)
		return
	}
	if len(batch.Events) > 200 {
		http.Error(w, "too many events", http.StatusBadRequest)
		return
	}
	accepted := 0
	for _, item := range batch.Events {
		var env event.Envelope
		if err := json.Unmarshal(item, &env); err != nil {
			http.Error(w, "invalid event", http.StatusBadRequest)
			return
		}
		if env.SchemaVersion != 1 || !shared.IsUUID(env.EventID) || !event.KnownType(env.EventType) {
			http.Error(w, "invalid event envelope", http.StatusBadRequest)
			return
		}
		if env.ServiceKey != "" && !event.ValidServiceKey(env.ServiceKey) {
			http.Error(w, "invalid service_key", http.StatusBadRequest)
			return
		}
		if env.EventType == event.TypeLogAppend && s.OnLogAppend != nil {
			var lp event.LogPayload
			_ = json.Unmarshal(env.Payload, &lp)
			s.OnLogAppend(env.ServiceKey, lp.Markdown, lp.Severity, env.OccurredAt, lp.Source)
		}
		if s.OnEvent != nil {
			s.OnEvent(env)
		}
		accepted++
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = w.Write([]byte(fmt.Sprintf(`{"accepted":%d}`, accepted)))
}

// Start listens until ctx is cancelled.
func (s *Server) Start(ctx context.Context, advertisePath string) error {
	ln, err := net.Listen("tcp", s.listen)
	if err != nil {
		return err
	}
	if advertisePath != "" {
		if err := writeAdvertise(advertisePath, "http://"+ln.Addr().String()); err != nil {
			s.log.Warn("local ingest advertise failed", "path", advertisePath, "err", err)
		} else {
			s.log.Info("local ingest advertised", "path", advertisePath, "url", "http://"+ln.Addr().String())
		}
	}
	s.http.BaseContext = func(net.Listener) context.Context { return ctx }
	go func() {
		<-ctx.Done()
		c, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		_ = s.http.Shutdown(c)
		if advertisePath != "" {
			_ = os.Remove(advertisePath)
		}
	}()
	err = s.http.Serve(ln)
	if errors.Is(err, http.ErrServerClosed) {
		return nil
	}
	return err
}

func writeAdvertise(path, url string) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o750); err != nil {
		return err
	}
	body, _ := json.Marshal(map[string]string{"url": url, "mode": "tee"})
	return os.WriteFile(path, body, 0o600)
}

// DefaultAdvertisePath is next to the spool database.
func DefaultAdvertisePath(spoolPath string) string {
	dir := filepath.Dir(spoolPath)
	if dir == "" || dir == "." {
		dir = "/var/lib/agentboard-client"
	}
	return filepath.Join(dir, "local-ingest.json")
}
