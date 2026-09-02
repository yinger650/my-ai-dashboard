package wrapd

import (
	"context"
	"io"
	"os"
	"strings"
	"sync"
	"time"

	"agentboard/internal/client/control"
	"agentboard/internal/event"
	"agentboard/internal/shared"
)

const (
	selfService = "board-client"
	maxBuf      = 256 * 1024
)

// EnqueueFunc writes an ingest event.
type EnqueueFunc func(eventType, serviceKey, runKey string, payload any)

// Manager tracks wrap jobs on the daemon side.
type Manager struct {
	Enqueue   EnqueueFunc
	Debug     func(string)
	Audit     func(runKey, serviceKey, status, summary string)
	Summarize func(runKey, text string)
	Now       func() time.Time

	mu   sync.Mutex
	jobs map[string]*job
	done map[string]string
}

type job struct {
	runKey   string
	summary  string
	logPath  string
	pid      int
	deadline time.Time
	terminal bool
	status   string
	buf      strings.Builder
	cancel   context.CancelFunc
}

// New returns an empty manager.
func New() *Manager {
	return &Manager{
		jobs: map[string]*job{},
		done: map[string]string{},
		Now:  time.Now,
	}
}

func (m *Manager) now() time.Time {
	if m.Now != nil {
		return m.Now()
	}
	return time.Now()
}

func (m *Manager) debug(msg string) {
	if m.Debug != nil {
		m.Debug(msg)
	}
}

// Handle implements control.Handler.
func (m *Manager) Handle(req control.Request) control.Response {
	switch req.Op {
	case "ping":
		return control.Response{OK: true}
	case "wrap_start":
		return m.start(req)
	case "wrap_stdout":
		return m.stdout(req)
	case "wrap_exit":
		return m.exit(req)
	default:
		return control.Response{Error: "unknown op " + req.Op}
	}
}

func (m *Manager) start(req control.Request) control.Response {
	if strings.TrimSpace(req.RunKey) == "" {
		return control.Response{Error: "run_key required"}
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, done := m.done[req.RunKey]; done {
		m.debug("wrap_start ignored; run already terminal: " + req.RunKey)
		return control.Response{OK: true}
	}
	if j, ok := m.jobs[req.RunKey]; ok && j.terminal {
		m.debug("wrap_start ignored; job terminal: " + req.RunKey)
		return control.Response{OK: true}
	}
	var deadline time.Time
	if req.TTLSeconds > 0 {
		deadline = m.now().Add(time.Duration(req.TTLSeconds) * time.Second)
	}
	j := &job{
		runKey:   req.RunKey,
		summary:  req.Summary,
		logPath:  req.LogPath,
		pid:      req.PID,
		deadline: deadline,
	}
	if old, ok := m.jobs[req.RunKey]; ok && old.cancel != nil {
		old.cancel()
	}
	m.jobs[req.RunKey] = j
	if m.Enqueue != nil {
		m.Enqueue(event.TypeRunTransition, selfService, req.RunKey, event.RunTransition{
			ServiceName: "Board Client",
			ServiceType: "daemon",
			Status:      "running",
			Summary:     req.Summary,
			StartedAt:   shared.FormatTime(m.now().UTC()),
			Metadata:    map[string]any{"pid": req.PID, "log_path": req.LogPath, "wrap": true},
		})
	}
	if req.LogPath != "" {
		ctx, cancel := context.WithCancel(context.Background())
		j.cancel = cancel
		go m.tail(ctx, req.RunKey, req.LogPath)
	}
	return control.Response{OK: true}
}

func (m *Manager) stdout(req control.Request) control.Response {
	if req.Chunk == "" {
		return control.Response{OK: true}
	}
	m.mu.Lock()
	j := m.jobs[req.RunKey]
	if j == nil || j.terminal {
		m.mu.Unlock()
		return control.Response{OK: true}
	}
	m.appendLocked(j, req.Chunk)
	text := j.buf.String()
	m.mu.Unlock()
	if m.Summarize != nil && strings.TrimSpace(text) != "" {
		m.Summarize(req.RunKey, text)
	}
	return control.Response{OK: true}
}

func (m *Manager) appendLocked(j *job, chunk string) {
	if j.buf.Len()+len(chunk) > maxBuf {
		keep := j.buf.String()
		if len(keep) > maxBuf/2 {
			keep = keep[len(keep)-maxBuf/2:]
		}
		j.buf.Reset()
		j.buf.WriteString(keep)
	}
	j.buf.WriteString(chunk)
}

func (m *Manager) exit(req control.Request) control.Response {
	code := 0
	if req.ExitCode != nil {
		code = *req.ExitCode
	}
	status := "succeeded"
	if code != 0 {
		status = "failed"
	}
	m.mu.Lock()
	if prev, ok := m.done[req.RunKey]; ok && event.IsTerminal(prev) {
		m.mu.Unlock()
		m.debug("wrap_exit skipped; already terminal " + req.RunKey + " " + prev)
		return control.Response{OK: true}
	}
	j := m.jobs[req.RunKey]
	if j != nil && j.terminal {
		m.done[req.RunKey] = j.status
		m.mu.Unlock()
		m.debug("wrap_exit skipped; job already terminal " + req.RunKey)
		return control.Response{OK: true}
	}
	summary := ""
	if j != nil {
		summary = j.summary
		j.terminal = true
		j.status = status
		if j.cancel != nil {
			j.cancel()
		}
	}
	m.done[req.RunKey] = status
	m.mu.Unlock()
	m.finish(req.RunKey, status, summary, code)
	return control.Response{OK: true}
}

func (m *Manager) finish(runKey, status, summary string, exitCode int) {
	finished := shared.FormatTime(m.now().UTC())
	if m.Enqueue != nil {
		meta := map[string]any{"wrap": true}
		if exitCode != 0 || status == "failed" {
			meta["exit_code"] = exitCode
		}
		m.Enqueue(event.TypeRunTransition, selfService, runKey, event.RunTransition{
			ServiceName: "Board Client",
			ServiceType: "daemon",
			Status:      status,
			Summary:     summary,
			FinishedAt:  finished,
			Metadata:    meta,
		})
	}
	if m.Audit != nil {
		m.Audit(runKey, selfService, status, summary)
	}
}

// TickTTL marks overdue jobs timed_out without killing the process.
func (m *Manager) TickTTL() {
	now := m.now()
	var due []string
	m.mu.Lock()
	for k, j := range m.jobs {
		if j.terminal || j.deadline.IsZero() || now.Before(j.deadline) {
			continue
		}
		j.terminal = true
		j.status = "timed_out"
		m.done[k] = "timed_out"
		if j.cancel != nil {
			j.cancel()
		}
		due = append(due, k)
	}
	m.mu.Unlock()
	for _, k := range due {
		m.mu.Lock()
		j := m.jobs[k]
		sum := ""
		if j != nil {
			sum = j.summary
		}
		m.mu.Unlock()
		m.finish(k, "timed_out", sum, 0)
	}
}

func (m *Manager) tail(ctx context.Context, runKey, path string) {
	var f *os.File
	var offset int64
	ticker := time.NewTicker(400 * time.Millisecond)
	defer ticker.Stop()
	defer func() {
		if f != nil {
			_ = f.Close()
		}
	}()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if f == nil {
				nf, err := os.Open(path)
				if err != nil {
					continue
				}
				f = nf
				offset = 0
			}
			st, err := f.Stat()
			if err != nil {
				_ = f.Close()
				f = nil
				continue
			}
			size := st.Size()
			if size < offset {
				_, _ = f.Seek(0, io.SeekStart)
				offset = 0
			}
			if size == offset {
				continue
			}
			buf := make([]byte, size-offset)
			n, err := f.ReadAt(buf, offset)
			if n > 0 {
				offset += int64(n)
				m.stdout(control.Request{Op: "wrap_stdout", RunKey: runKey, Chunk: string(buf[:n])})
			}
			if err != nil && err != io.EOF {
				_ = f.Close()
				f = nil
			}
		}
	}
}

// Terminal reports whether runKey already finished.
func (m *Manager) Terminal(runKey string) bool {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.done[runKey]; ok {
		return true
	}
	j := m.jobs[runKey]
	return j != nil && j.terminal
}
