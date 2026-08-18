package runner

import (
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"

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
