package runner

import (
	"context"
	"fmt"
	"strings"
	"sync"
	"time"

	"agentboard/internal/client/agent"
	"agentboard/internal/client/config"
	"agentboard/internal/client/control"
	"agentboard/internal/event"
	"agentboard/internal/summarize"
)

const auditCap = 500

// SetConfigPath records the yaml used for reload.
func (r *Runner) SetConfigPath(path string) {
	r.cfgPath = path
}

func (r *Runner) startControl(ctx context.Context, wg *sync.WaitGroup) error {
	if r.cfg == nil {
		return fmt.Errorf("no config")
	}
	srv, err := control.Listen(r.cfg.ControlSockPath(), r.handleControl)
	if err != nil {
		return err
	}
	wg.Add(1)
	go func() {
		defer wg.Done()
		if err := srv.Serve(ctx); err != nil && r.log != nil {
			r.log.Warn("control socket stopped", "err", err)
		}
	}()
	if r.log != nil {
		r.log.Info("control socket listening", "path", r.cfg.ControlSockPath())
	}
	return nil
}

func (r *Runner) handleControl(req control.Request) control.Response {
	switch req.Op {
	case "ping":
		return control.Response{OK: true}
	case "reload":
		if err := r.Reload(); err != nil {
			return control.Response{Error: err.Error()}
		}
		return control.Response{OK: true}
	default:
		if r.wrap == nil {
			return control.Response{Error: "wrap unavailable"}
		}
		return r.wrap.Handle(req)
	}
}

// Reload re-reads yaml and recompiles status probes without stopping collect.
func (r *Runner) Reload() error {
	if r.cfgPath == "" {
		return fmt.Errorf("no config path")
	}
	c, err := config.Load(r.cfgPath)
	if err != nil {
		return err
	}
	r.mu.Lock()
	r.cfg = c
	if r.snd != nil {
		r.snd.Token = c.Token()
		r.snd.BaseURL = strings.TrimRight(c.Server.URL, "/")
	}
	r.mu.Unlock()
	go r.compileStatusProbes(context.Background())
	return nil
}

func (r *Runner) noteTaskDone(runKey, serviceKey, status, summary string) {
	if runKey == "" || !event.IsTerminal(status) {
		return
	}
	r.mu.Lock()
	if r.proj == nil {
		r.proj = agent.NewState()
	}
	for _, k := range r.proj.AuditRunKeys {
		if k == runKey {
			r.mu.Unlock()
			return
		}
	}
	r.proj.AuditRunKeys = append(r.proj.AuditRunKeys, runKey)
	if len(r.proj.AuditRunKeys) > auditCap {
		r.proj.AuditRunKeys = r.proj.AuditRunKeys[len(r.proj.AuditRunKeys)-auditCap:]
	}
	r.saveInspectState()
	r.mu.Unlock()
	line := "完成 task · " + serviceKey + " · " + status + " · " + clipRunes(summary, 80)
	r.emitSelfLog(line, "info")
}

func mergeAuditKeys(a, b []string) []string {
	seen := map[string]struct{}{}
	var out []string
	for _, src := range [][]string{a, b} {
		for _, k := range src {
			if k == "" {
				continue
			}
			if _, ok := seen[k]; ok {
				continue
			}
			seen[k] = struct{}{}
			out = append(out, k)
		}
	}
	if len(out) > auditCap {
		out = out[len(out)-auditCap:]
	}
	return out
}

func (r *Runner) summarizeWrap(runKey, text string) {
	if strings.TrimSpace(text) == "" {
		return
	}
	r.mu.Lock()
	last := r.lastWrapSum[runKey]
	if !last.IsZero() && time.Since(last) < 30*time.Second {
		r.mu.Unlock()
		return
	}
	r.lastWrapSum[runKey] = time.Now()
	r.mu.Unlock()
	md := summarize.Logs("wrap "+runKey, []string{text})
	r.enqueue(event.TypeLogAppend, selfServiceKey, runKey, event.LogPayload{
		Markdown: md, Severity: "info", Source: "wrap",
	})
}
