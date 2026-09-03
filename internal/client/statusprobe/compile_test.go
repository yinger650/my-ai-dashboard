package statusprobe

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"agentboard/internal/client/aiprovider"
	"agentboard/internal/client/config"
	"agentboard/internal/client/probe"
)

type stubProvider struct {
	text     string
	err      error
	slow     time.Duration
	n        atomic.Int32
	lastTask atomic.Value
}

func (s *stubProvider) Name() string { return "stub" }

func (s *stubProvider) Run(ctx context.Context, req aiprovider.Request) (aiprovider.Result, error) {
	s.n.Add(1)
	s.lastTask.Store(req.Task)
	if s.slow > 0 {
		select {
		case <-ctx.Done():
			return aiprovider.Result{}, ctx.Err()
		case <-time.After(s.slow):
		}
	}
	if s.err != nil {
		return aiprovider.Result{}, s.err
	}
	return aiprovider.Result{Text: s.text}, nil
}

func validScript() string {
	return "#!/bin/sh\nprintf '%s\\n' '{\"state\":\"running\",\"summary\":\"ok\",\"severity\":\"normal\",\"statuses\":[{\"key\":\"gpu_util\",\"value\":\"41\"}]}'\n"
}

func TestPrepareHandwrittenSkipsAI(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "hand.sh")
	if err := os.WriteFile(script, []byte(validScript()), 0o755); err != nil {
		t.Fatal(err)
	}
	prov := &stubProvider{text: "should not run"}
	c := &Compiler{Dir: filepath.Join(dir, "probes"), Provider: prov, AIEnabled: true}
	got := c.Prepare(context.Background(), []config.StatusProbe{{
		Key: "gpu", Command: []string{script},
	}})
	if len(got) != 1 || got[0].Command[0] != script {
		t.Fatalf("got %+v", got)
	}
	if prov.n.Load() != 0 {
		t.Fatal("handwritten must skip AI")
	}
}

func TestPrepareCompilesAndReusesHash(t *testing.T) {
	dir := t.TempDir()
	prov := &stubProvider{text: validScript()}
	c := &Compiler{Dir: dir, Provider: prov, AIEnabled: true}
	probes := []config.StatusProbe{{Key: "gpu", Intent: "NVIDIA GPU 利用率 0-100"}}
	got := c.Prepare(context.Background(), probes)
	if len(got) != 1 {
		t.Fatalf("ready=%d", len(got))
	}
	if err := probe.CheckScript(got[0].Command[0]); err != nil {
		t.Fatal(err)
	}
	if prov.n.Load() != 1 {
		t.Fatalf("calls=%d", prov.n.Load())
	}
	got2 := c.Prepare(context.Background(), probes)
	if len(got2) != 1 {
		t.Fatal("reuse")
	}
	if prov.n.Load() != 1 {
		t.Fatalf("hash hit should skip AI, calls=%d", prov.n.Load())
	}
}

func TestPrepareServiceUsesServicePrompt(t *testing.T) {
	dir := t.TempDir()
	prov := &stubProvider{text: validScript()}
	c := &Compiler{Dir: dir, Provider: prov, AIEnabled: true}
	got := c.Prepare(context.Background(), []config.StatusProbe{{
		Key: "docker-nginx", Kind: config.StatusProbeService, Name: "容器 Nginx",
		Intent: "通过 docker exec 检查 nginx",
	}})
	if len(got) != 1 || got[0].Kind != config.StatusProbeService || got[0].Name != "容器 Nginx" {
		t.Fatalf("ready=%+v", got)
	}
	if task, _ := prov.lastTask.Load().(string); task != "service_probe_script" {
		t.Fatalf("task=%q", task)
	}
}

func TestPrepareHTTPCompilesCachesAndValidates(t *testing.T) {
	dir := t.TempDir()
	prov := &stubProvider{text: "```json\n" + `{"url":"http://127.0.0.1:18080/health","method":"GET","expect_status":[200,204]}` + "\n```"}
	c := &Compiler{Dir: dir, Provider: prov, AIEnabled: true}
	p := config.StatusProbe{
		Key: "local-health", Kind: config.StatusProbeHTTP, Name: "本机服务",
		Intent: "检查 18080 health", TTLSeconds: 180,
	}
	got := c.Prepare(context.Background(), []config.StatusProbe{p})
	if len(got) != 1 || got[0].HTTP == nil {
		t.Fatalf("ready=%+v", got)
	}
	if got[0].HTTP.ServiceKey != p.Key || got[0].HTTP.Name != p.Name || len(got[0].HTTP.ExpectStatus) != 2 {
		t.Fatalf("target=%+v", got[0].HTTP)
	}
	if task, _ := prov.lastTask.Load().(string); task != "http_probe_config" {
		t.Fatalf("task=%q", task)
	}
	if again := c.Prepare(context.Background(), []config.StatusProbe{p}); len(again) != 1 || prov.n.Load() != 1 {
		t.Fatalf("cache miss ready=%+v calls=%d", again, prov.n.Load())
	}
}

func TestPrepareHTTPRejectsUnsafeAndReusesOld(t *testing.T) {
	dir := t.TempDir()
	prov := &stubProvider{text: `{"url":"http://127.0.0.1:18080/health","method":"GET","expect_status":[200]}`}
	c := &Compiler{Dir: dir, Provider: prov, AIEnabled: true}
	p := config.StatusProbe{Key: "health", Kind: config.StatusProbeHTTP, Intent: "health"}
	if got := c.Prepare(context.Background(), []config.StatusProbe{p}); len(got) != 1 {
		t.Fatalf("initial=%+v", got)
	}
	p.Intent = "changed"
	prov.text = `{"url":"http://user:secret@127.0.0.1/health","method":"POST","expect_status":[999]}`
	got := c.Prepare(context.Background(), []config.StatusProbe{p})
	if len(got) != 1 || got[0].HTTP == nil || got[0].HTTP.URL != "http://127.0.0.1:18080/health" {
		t.Fatalf("old target not reused: %+v", got)
	}
}

func TestIntentKindChangeRecompiles(t *testing.T) {
	dir := t.TempDir()
	prov := &stubProvider{text: validScript()}
	c := &Compiler{Dir: dir, Provider: prov, AIEnabled: true}
	p := config.StatusProbe{Key: "check", Intent: "one"}
	if len(c.Prepare(context.Background(), []config.StatusProbe{p})) != 1 {
		t.Fatal("metric prepare failed")
	}
	p.Kind = config.StatusProbeService
	if len(c.Prepare(context.Background(), []config.StatusProbe{p})) != 1 {
		t.Fatal("service prepare failed")
	}
	if prov.n.Load() != 2 {
		t.Fatalf("kind change should compile, calls=%d", prov.n.Load())
	}
}

func TestPrepareKeepsOldOnBadScript(t *testing.T) {
	dir := t.TempDir()
	script := filepath.Join(dir, "gpu.sh")
	if err := os.WriteFile(script, []byte(validScript()), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "gpu.meta.json"), []byte(`{"hash":"old"}`), 0o600); err != nil {
		t.Fatal(err)
	}
	old, _ := os.ReadFile(script)
	prov := &stubProvider{text: "#!/bin/sh\necho not-json\n"}
	var notices []string
	c := &Compiler{
		Dir: dir, Provider: prov, AIEnabled: true,
		Notice: func(_, md string) { notices = append(notices, md) },
	}
	got := c.Prepare(context.Background(), []config.StatusProbe{{Key: "gpu", Intent: "changed intent"}})
	if len(got) != 1 {
		t.Fatalf("should keep old, got %d notices=%v", len(got), notices)
	}
	now, _ := os.ReadFile(script)
	if string(now) != string(old) {
		t.Fatalf("old script overwritten:\n%s", now)
	}
	if len(notices) == 0 {
		t.Fatal("expected notice")
	}
}

func TestPrepareAIDisabledNoScript(t *testing.T) {
	dir := t.TempDir()
	var notices []string
	c := &Compiler{Dir: dir, AIEnabled: false, Notice: func(code, _ string) { notices = append(notices, code) }}
	got := c.Prepare(context.Background(), []config.StatusProbe{{Key: "gpu", Intent: "util"}})
	if len(got) != 0 {
		t.Fatalf("got %+v", got)
	}
	if len(notices) != 1 || notices[0] != "status_probe_skipped" {
		t.Fatalf("notices=%v", notices)
	}
}

func TestPrepareDoesNotBlockOnSlowProvider(t *testing.T) {
	dir := t.TempDir()
	prov := &stubProvider{text: validScript(), slow: 400 * time.Millisecond}
	c := &Compiler{Dir: dir, Provider: prov, AIEnabled: true}
	done := make(chan []Ready, 1)
	go func() {
		done <- c.Prepare(context.Background(), []config.StatusProbe{{Key: "gpu", Intent: "util"}})
	}()
	select {
	case <-done:
		t.Fatal("prepare returned too quickly; this test asserts caller can proceed concurrently")
	case <-time.After(50 * time.Millisecond):
	}
	select {
	case got := <-done:
		if len(got) != 1 {
			t.Fatalf("ready=%d", len(got))
		}
	case <-time.After(2 * time.Second):
		t.Fatal("prepare hung")
	}
}

func TestExtractScript(t *testing.T) {
	got := ExtractScript("```sh\n#!/bin/sh\necho hi\n```\n")
	if !strings.Contains(got, "echo hi") {
		t.Fatalf("%q", got)
	}
}

func TestValidateGeneratedRejectsNetworkAndToken(t *testing.T) {
	for _, body := range []string{
		"#!/bin/sh\ncurl http://example.com",
		"#!/bin/sh\nwget http://example.com",
		"#!/bin/sh\necho \"$AGENTBOARD_TOKEN\"",
		"#!/bin/sh\necho abp_m_secret",
		"#!/bin/sh\necho /ingest/events",
	} {
		if err := validateGenerated(body); err == nil {
			t.Fatalf("expected rejection: %s", body)
		}
	}
}

func TestNumericMeta(t *testing.T) {
	m := NumericMeta("gpu", []probe.Status{
		{Key: "gpu_util", Value: "41.5"},
		{Key: "note", Value: "ok"},
	})
	if m["gpu_util"] != 41.5 {
		t.Fatalf("%v", m)
	}
	if _, ok := m["note"]; ok {
		t.Fatal("non-numeric leaked")
	}
}
