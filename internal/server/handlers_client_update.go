package server

import (
	"context"
	"crypto/subtle"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/go-chi/chi/v5"

	"agentboard/internal/client/update"
)

func (s *Server) clientUpdatePath(name string) (string, bool) {
	name = filepath.Base(strings.TrimSpace(name))
	if name == "" || name != filepath.Clean(name) || strings.Contains(name, "/") || strings.Contains(name, "\\") {
		return "", false
	}
	if !update.AllowedNames[name] {
		return "", false
	}
	dir := s.cfg.ClientUpdateDir
	if dir == "" {
		return "", false
	}
	return filepath.Join(dir, name), true
}

func (s *Server) handleGetClientUpdate(w http.ResponseWriter, r *http.Request) {
	path, ok := s.clientUpdatePath(chi.URLParam(r, "name"))
	if !ok {
		http.NotFound(w, r)
		return
	}
	name := filepath.Base(path)
	f, err := os.Open(path)
	if err != nil {
		http.NotFound(w, r)
		return
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil || st.IsDir() {
		http.NotFound(w, r)
		return
	}
	switch name {
	case "manifest.json":
		w.Header().Set("Content-Type", "application/json; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
	case "SHA256SUMS":
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.Header().Set("Cache-Control", "no-store")
	default:
		w.Header().Set("Content-Type", "application/octet-stream")
		w.Header().Set("Cache-Control", "public, max-age=300")
	}
	w.Header().Set("X-Content-Type-Options", "nosniff")
	http.ServeContent(w, r, name, st.ModTime(), f)
}

func (s *Server) handlePutClientUpdate(w http.ResponseWriter, r *http.Request) {
	if !s.clientUpdateTokenOK(r) {
		http.Error(w, "forbidden", http.StatusForbidden)
		return
	}
	name := chi.URLParam(r, "name")
	dest, ok := s.clientUpdatePath(name)
	if !ok {
		http.NotFound(w, r)
		return
	}
	name = filepath.Base(dest)
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	limit := int64(update.MaxBinaryBytes)
	data, err := io.ReadAll(io.LimitReader(r.Body, limit+1))
	if err != nil {
		http.Error(w, "read failed", http.StatusBadRequest)
		return
	}
	if int64(len(data)) > limit {
		http.Error(w, "too large", http.StatusRequestEntityTooLarge)
		return
	}
	if name == "manifest.json" && !utf8.Valid(data) {
		http.Error(w, "invalid manifest", http.StatusBadRequest)
		return
	}
	mode := os.FileMode(0o644)
	if strings.HasPrefix(name, "board-client-") {
		mode = 0o755
	}
	tmp, err := os.CreateTemp(filepath.Dir(dest), "."+name+".")
	if err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	tmpName := tmp.Name()
	defer os.Remove(tmpName)
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if err := tmp.Chmod(mode); err != nil {
		_ = tmp.Close()
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if err := tmp.Close(); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	if err := os.Rename(tmpName, dest); err != nil {
		http.Error(w, "internal error", http.StatusInternalServerError)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) clientUpdateTokenOK(r *http.Request) bool {
	want := strings.TrimSpace(s.cfg.ClientUpdateToken)
	if want == "" {
		return false
	}
	got := strings.TrimSpace(r.Header.Get("X-AgentBoard-Update-Token"))
	if got == "" {
		got = bearerToken(r.Header.Get("Authorization"))
	}
	if got == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(got), []byte(want)) == 1
}

func bearerToken(h string) string {
	const p = "Bearer "
	if len(h) < len(p) || !strings.EqualFold(h[:len(p)], p) {
		return ""
	}
	return strings.TrimSpace(h[len(p):])
}

// SyncClientUpdates pulls the rolling GitHub (or configured) release into the local mirror.
func (s *Server) SyncClientUpdates(ctx context.Context) error {
	if s.cfg.ClientUpdateDir == "" || s.cfg.ClientUpdateSource == "" {
		return nil
	}
	if err := os.MkdirAll(s.cfg.ClientUpdateDir, 0o755); err != nil {
		return err
	}
	up := update.New(s.cfg.ClientUpdateSource, update.Info{Version: "board-server", Commit: "sync"}, 10*time.Minute)
	_, err := up.MirrorTo(ctx, s.cfg.ClientUpdateDir)
	return err
}

func (s *Server) RunClientUpdateSync(ctx context.Context) {
	if !s.cfg.ClientUpdateSync {
		return
	}
	run := func() {
		c, cancel := context.WithTimeout(context.Background(), 15*time.Minute)
		err := s.SyncClientUpdates(c)
		cancel()
		if err != nil {
			s.log.Warn("client-update mirror sync failed", "err", err, "source", s.cfg.ClientUpdateSource)
			return
		}
		s.log.Info("client-update mirror synced", "dir", s.cfg.ClientUpdateDir)
	}
	select {
	case <-ctx.Done():
		return
	case <-time.After(20 * time.Second):
		run()
	}
	t := time.NewTicker(time.Hour)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			run()
		}
	}
}
