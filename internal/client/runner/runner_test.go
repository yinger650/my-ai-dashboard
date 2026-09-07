package runner

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"agentboard/internal/client/aiinspect"
	"agentboard/internal/client/config"
	"agentboard/internal/client/spool"
	"agentboard/internal/event"
)

func TestEmitHTTPQueuesStateStatusAndFailureLog(t *testing.T) {
	up := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}))
	defer up.Close()
	down := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer down.Close()

	dir := t.TempDir()
	t.Setenv("ABP_MACHINE_TOKEN", "abp_m_test")
	cfgPath := filepath.Join(dir, "client.yaml")
	src := `version: 1
server:
  url: "http://127.0.0.1:9"
machine:
  key: "aliyun-web"
storage:
  spool_path: "` + filepath.Join(dir, "spool.db") + `"
collectors:
  http:
    enabled: true
    timeout: 2s
    targets:
      - service_key: site-up
        name: Up
        url: "` + up.URL + `"
      - service_key: site-down
        name: Down
        url: "` + down.URL + `"
`
	if err := os.WriteFile(cfgPath, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	sp, err := spool.Open(cfg.Storage.SpoolPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sp.Close() })

	r := New(cfg, sp, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})))
	r.emitHTTP()

	batch, err := sp.Batch(50, 256*1024)
	if err != nil {
		t.Fatal(err)
	}
	types := map[string]int{}
	keys := map[string]int{}
	for _, q := range batch {
		var env event.Envelope
		if err := json.Unmarshal([]byte(q.Payload), &env); err != nil {
			t.Fatal(err)
		}
		types[env.EventType]++
		if env.ServiceKey != "" {
			keys[env.ServiceKey]++
		}
	}
	if types[event.TypeServiceState] != 2 {
		t.Fatalf("service.state count = %d types=%v", types[event.TypeServiceState], types)
	}
	if types[event.TypeStatusUpsert] != 2 {
		t.Fatalf("status.upsert count = %d", types[event.TypeStatusUpsert])
	}
	if types[event.TypeLogAppend] != 1 {
		t.Fatalf("log.append count = %d (want 1 for first failure only)", types[event.TypeLogAppend])
	}
	if keys["site-up"] == 0 || keys["site-down"] == 0 {
		t.Fatalf("service keys = %v", keys)
	}

	r.emitHTTP()
	batch2, err := sp.Batch(50, 256*1024)
	if err != nil {
		t.Fatal(err)
	}
	logs := 0
	for _, q := range batch2 {
		var env event.Envelope
		_ = json.Unmarshal([]byte(q.Payload), &env)
		if env.EventType == event.TypeLogAppend {
			logs++
		}
	}
	if logs != 1 {
		t.Fatalf("second probe should not add another failure log, logs=%d batch=%d", logs, len(batch2))
	}
}

func TestCollectAndProjectEnqueuesInspectWithoutOwnLogs(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ABP_MACHINE_TOKEN", "abp_m_test")
	cfgPath := filepath.Join(dir, "client.yaml")
	src := `version: 1
server:
  url: "http://127.0.0.1:9"
machine:
  key: "home-server"
storage:
  spool_path: "` + filepath.Join(dir, "spool.db") + `"
collectors:
  cpu: true
  memory: true
  ports:
    enabled: true
  docker:
    enabled: true
  cron:
    enabled: true
  nginx:
    enabled: true
`
	if err := os.WriteFile(cfgPath, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg, err := config.Load(cfgPath)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	sp, err := spool.Open(cfg.Storage.SpoolPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sp.Close() })

	r := New(cfg, sp, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})))
	r.runCmd = func(name string, args ...string) ([]byte, error) {
		if name == "ss" {
			return []byte("tcp LISTEN 0 128 0.0.0.0:80 0.0.0.0:* users:((\"nginx\",pid=1,fd=6))\n"), nil
		}
		return nil, os.ErrNotExist
	}
	r.collectAndProject()

	batch, err := sp.Batch(100, 256*1024)
	if err != nil {
		t.Fatal(err)
	}
	types := map[string]int{}
	inspectLogs := 0
	hasInspect := false
	hasHeartbeat := false
	for _, q := range batch {
		var env event.Envelope
		if err := json.Unmarshal([]byte(q.Payload), &env); err != nil {
			t.Fatal(err)
		}
		types[env.EventType]++
		if env.ServiceKey == "host-inspect" {
			hasInspect = true
			if env.EventType == event.TypeLogAppend {
				inspectLogs++
			}
		}
		if env.EventType == event.TypeHeartbeat {
			hasHeartbeat = true
		}
	}
	if !hasHeartbeat {
		t.Fatalf("missing heartbeat: %v", types)
	}
	if !hasInspect {
		t.Fatal("missing host-inspect events")
	}
	if inspectLogs != 0 {
		t.Fatalf("host-inspect must not append logs, got %d", inspectLogs)
	}
	if types[event.TypePortSnapshot] != 1 {
		t.Fatalf("port snapshot = %d types=%v", types[event.TypePortSnapshot], types)
	}
}

func TestEmitProbeJSON(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "p.sh")
	body := "#!/bin/sh\nprintf '%s\\n' '{\"state\":\"running\",\"summary\":\"ok\",\"severity\":\"normal\",\"statuses\":[{\"key\":\"load1\",\"value\":\"0.1\"}],\"pinned_markdown\":\"load 0.1\"}'\n"
	if err := os.WriteFile(script, []byte(body), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(script, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ABP_MACHINE_TOKEN", "abp_m_test")
	cfgPath := filepath.Join(dir, "client.yaml")
	src := `version: 1
server:
  url: "http://127.0.0.1:9"
machine:
  key: "home-server"
storage:
  spool_path: "` + filepath.Join(dir, "spool.db") + `"
collectors:
  probes:
    enabled: true
    scripts:
      - service_key: load
        name: Load
        command: ["` + script + `"]
        format: json
        timeout: 5s
`
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
	r.emitProbes()
	batch, err := sp.Batch(20, 256*1024)
	if err != nil {
		t.Fatal(err)
	}
	types := map[string]int{}
	for _, q := range batch {
		var env event.Envelope
		_ = json.Unmarshal([]byte(q.Payload), &env)
		types[env.EventType]++
		if env.ServiceKey != "" && env.ServiceKey != "load" {
			t.Fatalf("unexpected key %s", env.ServiceKey)
		}
	}
	if types[event.TypeServiceState] != 1 || types[event.TypeStatusUpsert] != 1 || types[event.TypeLogPin] != 1 {
		t.Fatalf("types %v", types)
	}
}

func TestEmitAISummariesCommandProvider(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "fake-agent.sh")
	srcScript := filepath.Join("..", "aiinspect", "testdata", "fake-agent.sh")
	b, err := os.ReadFile(srcScript)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(script, b, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ABP_MACHINE_TOKEN", "abp_m_test")
	cfgPath := filepath.Join(dir, "client.yaml")
	yaml := `version: 1
server:
  url: "http://127.0.0.1:9"
machine:
  key: "home-server"
storage:
  spool_path: "` + filepath.Join(dir, "spool.db") + `"
ai:
  enabled: true
  provider: command
  command: ["` + script + `"]
  timeout: 5s
  max_calls_per_day: 48
  summarize:
    - source: agent_logs
      service_key: ai-agent-digest
      name: Agent 日志总结
      interval: 1s
      min_new_logs: 3
`
	if err := os.WriteFile(cfgPath, []byte(yaml), 0o644); err != nil {
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
	for _, md := range []string{"start task", "error failed", "still running"} {
		_ = r.logBuf.Append(aiinspect.Entry{Markdown: md, Severity: "info", Source: "cursor"})
	}
	r.emitAISummaries()
	batch, err := sp.Batch(20, 256*1024)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, q := range batch {
		var env event.Envelope
		_ = json.Unmarshal([]byte(q.Payload), &env)
		if env.EventType == event.TypeLogPin && env.ServiceKey == "ai-agent-digest" {
			found = true
			if !strings.Contains(string(env.Payload), "stub") && !strings.Contains(string(env.Payload), "摘要") {
				t.Fatalf("pin body %s", env.Payload)
			}
		}
	}
	if !found {
		t.Fatal("expected ai-agent-digest pin")
	}
}

func TestIngestProjectCopyQueuesProjService(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ABP_MACHINE_TOKEN", "abp_m_test")
	cfgPath := filepath.Join(dir, "client.yaml")
	yaml := `version: 1
server:
  url: "http://127.0.0.1:9"
machine:
  key: "devbox"
storage:
  spool_path: "` + filepath.Join(dir, "spool.db") + `"
`
	if err := os.WriteFile(cfgPath, []byte(yaml), 0o644); err != nil {
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
	raw, _ := json.Marshal(event.ServiceState{
		Name: "Cursor Agent", Type: "agent", State: "running", Severity: "normal",
		Metadata: map[string]any{"workspace": "/work/my-ai-dashboard", "project": "my-ai-dashboard"},
	})
	r.ingestProjectCopy(event.Envelope{EventType: event.TypeServiceState, ServiceKey: "cursor", Payload: raw})
	batch, err := sp.Batch(20, 256*1024)
	if err != nil {
		t.Fatal(err)
	}
	keys := map[string]int{}
	types := map[string]int{}
	for _, q := range batch {
		var env event.Envelope
		_ = json.Unmarshal([]byte(q.Payload), &env)
		keys[env.ServiceKey]++
		types[env.EventType]++
	}
	if keys["proj-my-ai-dashboard"] < 1 {
		t.Fatalf("keys=%v types=%v", keys, types)
	}
	if types[event.TypeServiceState] < 1 || types[event.TypeStatusUpsert] < 1 {
		t.Fatalf("types=%v", types)
	}
}

func TestMaybeNotifyNewFeatures(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("ABP_MACHINE_TOKEN", "abp_m_test")
	cfgPath := filepath.Join(dir, "client.yaml")
	yaml := `version: 1
server:
  url: "http://127.0.0.1:9"
machine:
  key: "home-server"
storage:
  spool_path: "` + filepath.Join(dir, "spool.db") + `"
collectors:
  cpu: true
  memory: true
`
	if err := os.WriteFile(cfgPath, []byte(yaml), 0o644); err != nil {
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
	r.Build.Version = "0.1.11"
	r.maybeNotifyNewFeatures()

	raw, ok, err := sp.GetState(config.SeenFeaturesKey)
	if err != nil || !ok {
		t.Fatalf("baseline seen missing ok=%v err=%v", ok, err)
	}
	seen := config.ParseSeenIDs(raw)
	if !containsID(seen, "cpu") || containsID(seen, "ai.discover") {
		t.Fatalf("baseline=%v", seen)
	}

	batch, err := sp.Batch(50, 256*1024)
	if err != nil {
		t.Fatal(err)
	}
	var log, status bool
	for _, q := range batch {
		var env event.Envelope
		_ = json.Unmarshal([]byte(q.Payload), &env)
		if env.EventType == event.TypeLogAppend && env.ServiceKey == "board-client" && strings.Contains(string(env.Payload), "AI 主机巡检") {
			log = true
			if !strings.Contains(string(env.Payload), "config tui") {
				t.Fatalf("log missing command: %s", env.Payload)
			}
		}
		if env.EventType == event.TypeStatusUpsert && strings.Contains(string(env.Payload), "config_new_features") {
			status = true
		}
	}
	if !log || !status {
		t.Fatalf("log=%v status=%v n=%d", log, status, len(batch))
	}

	if err := sp.SetState(config.SeenFeaturesKey, config.EncodeSeenIDs(config.AllCatalogIDs())); err != nil {
		t.Fatal(err)
	}
	r2 := New(cfg, sp, slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: slog.LevelError})))
	r2.maybeNotifyNewFeatures()
	batch2, _ := sp.Batch(50, 256*1024)
	var extra int
	for _, q := range batch2 {
		var env event.Envelope
		_ = json.Unmarshal([]byte(q.Payload), &env)
		if env.EventType == event.TypeLogAppend && strings.Contains(string(env.Payload), "有新功能可配置") {
			extra++
		}
	}
	if extra != 1 {
		t.Fatalf("expected one notice log, got %d", extra)
	}
}

func containsID(ids []string, want string) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}
