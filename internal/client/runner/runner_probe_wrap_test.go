package runner

import (
	"context"
	"encoding/json"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agentboard/internal/client/aiprovider"
	"agentboard/internal/client/config"
	"agentboard/internal/client/spool"
	"agentboard/internal/event"
)

type slowProvider struct {
	text string
	wait time.Duration
}

func (s slowProvider) Name() string { return "slow" }
func (s slowProvider) Run(ctx context.Context, _ aiprovider.Request) (aiprovider.Result, error) {
	select {
	case <-ctx.Done():
		return aiprovider.Result{}, ctx.Err()
	case <-time.After(s.wait):
	}
	return aiprovider.Result{Text: s.text}, nil
}

func testRunner(t *testing.T, yamlBody string) *Runner {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("ABP_MACHINE_TOKEN", "abp_m_test")
	cfgPath := filepath.Join(dir, "client.yaml")
	spoolPath := filepath.Join(dir, "spool.db")
	src := strings.ReplaceAll(yamlBody, "SPOOL", spoolPath)
	if err := os.WriteFile(cfgPath, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatal(err)
	}
	sp, err := spool.Open(cfg.Storage.SpoolPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sp.Close() })
	r := New(cfg, sp, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})))
	r.SetConfigPath(cfgPath)
	r.runCmd = func(string, ...string) ([]byte, error) { return nil, os.ErrNotExist }
	return r
}

func drainEnvs(t *testing.T, r *Runner) []event.Envelope {
	t.Helper()
	batch, err := r.sp.Batch(200, 1024*1024)
	if err != nil {
		t.Fatal(err)
	}
	var out []event.Envelope
	for _, q := range batch {
		var env event.Envelope
		if err := json.Unmarshal([]byte(q.Payload), &env); err != nil {
			t.Fatal(err)
		}
		out = append(out, env)
	}
	return out
}

func TestCompileStatusProbesDoesNotBlockCollect(t *testing.T) {
	script := "#!/bin/sh\nprintf '%s\\n' '{\"state\":\"running\",\"statuses\":[{\"key\":\"gpu_util\",\"value\":\"1\"}]}'\n"
	r := testRunner(t, `version: 1
server:
  url: "http://127.0.0.1:9"
machine:
  key: "home-server"
  status_probes:
    - key: gpu
      intent: "util"
storage:
  spool_path: "SPOOL"
collectors:
  filesystems:
    enabled: false
  disk_io:
    enabled: false
  network:
    enabled: false
  ports:
    enabled: false
  docker:
    enabled: false
  cron:
    enabled: false
  nginx:
    enabled: false
ai:
  enabled: true
  provider: command
  command: ["/bin/true"]
`)
	r.provider = slowProvider{text: script, wait: 1500 * time.Millisecond}
	done := make(chan struct{})
	go func() {
		r.compileStatusProbes(context.Background())
		close(done)
	}()
	r.CollectOnce()
	select {
	case <-done:
		t.Fatal("compile finished before collect; expected async overlap")
	default:
	}
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatal("compile hung")
	}
}

func TestStatusProbeNumbersEnterHeartbeat(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "gpu.sh")
	body := "#!/bin/sh\nprintf '%s\\n' '{\"state\":\"running\",\"summary\":\"ok\",\"severity\":\"normal\",\"statuses\":[{\"key\":\"gpu_util\",\"value\":\"41\"},{\"key\":\"note\",\"value\":\"ok\"}]}'\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	r := testRunner(t, `version: 1
server:
  url: "http://127.0.0.1:9"
machine:
  key: "home-server"
  status_probes:
    - key: gpu
      command: ["`+script+`"]
storage:
  spool_path: "SPOOL"
`)
	r.compileStatusProbes(context.Background())
	r.emitStatusProbes()
	r.collectAndProject()
	found := false
	for _, env := range drainEnvs(t, r) {
		if env.EventType != event.TypeHeartbeat {
			continue
		}
		var hb event.Heartbeat
		if err := json.Unmarshal(env.Payload, &hb); err != nil {
			t.Fatal(err)
		}
		n, ok := hb.Metadata["gpu_util"].(float64)
		if !ok || n != 41 {
			t.Fatalf("metadata=%v", hb.Metadata)
		}
		if _, ok := hb.Metadata["note"]; ok {
			t.Fatal("non-numeric should not enter heartbeat")
		}
		found = true
	}
	if !found {
		t.Fatal("missing heartbeat metadata")
	}
}

func TestIngestTerminalWritesBoardClientAuditOnce(t *testing.T) {
	r := testRunner(t, `version: 1
server:
  url: "http://127.0.0.1:9"
machine:
  key: "devbox"
storage:
  spool_path: "SPOOL"
`)
	raw, _ := json.Marshal(event.RunTransition{
		ServiceName: "Cursor", ServiceType: "agent",
		Status: "succeeded", Summary: "实现 wrap",
		Metadata: map[string]any{"workspace": "/work/agentboard"},
	})
	env := event.Envelope{EventType: event.TypeRunTransition, ServiceKey: "cursor", RunKey: "rk-audit-1", Payload: raw}
	r.ingestProjectCopy(env)
	r.ingestProjectCopy(env)
	var projRun, audits int
	for _, e := range drainEnvs(t, r) {
		if e.EventType == event.TypeRunTransition && e.ServiceKey == "proj-agentboard" && e.RunKey == "rk-audit-1" {
			projRun++
		}
		if e.EventType == event.TypeLogAppend && e.ServiceKey == "board-client" {
			var lp event.LogPayload
			_ = json.Unmarshal(e.Payload, &lp)
			if strings.Contains(lp.Markdown, "完成 task · proj-agentboard · succeeded") {
				audits++
			}
		}
	}
	if projRun < 1 {
		t.Fatalf("expected proj-* run, projRun=%d", projRun)
	}
	if audits != 1 {
		t.Fatalf("audit lines=%d", audits)
	}
}
