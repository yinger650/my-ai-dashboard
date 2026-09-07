package cfgui

import (
	"bufio"
	"context"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agentboard/internal/client/aiprovider"
	"agentboard/internal/client/config"
	"agentboard/internal/client/spool"
)

func TestSaveWritesYAML(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "client.yaml")
	src := `version: 1
server:
  url: "https://board.yinger650.com"
  machine_token: "abp_m_old"
machine:
  key: "home-server"
storage:
  spool_path: "` + filepath.Join(dir, "spool.db") + `"
collectors:
  cpu: true
`
	if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	ed := config.Edit{
		URL:          "https://board.yinger650.com",
		MachineKey:   "home-server",
		Token:        "abp_m_ui_token_value",
		Features:     map[string]bool{"cpu": true, "memory": true},
		WriteProbes:  true,
		StatusProbes: []config.StatusProbe{{Key: "gpu", Intent: "util"}},
	}
	if err := SaveAndReload(p, ed); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	text := string(got)
	if !strings.Contains(text, "abp_m_ui_token_value") {
		t.Fatalf("token not saved: %s", text)
	}
	if !strings.Contains(text, "key: gpu") {
		t.Fatalf("probe not saved: %s", text)
	}
	if !strings.Contains(text, "memory: true") {
		t.Fatalf("memory not enabled: %s", text)
	}
	sp, err := spool.Open(filepath.Join(dir, "spool.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer sp.Close()
	raw, ok, err := sp.GetState(config.SeenFeaturesKey)
	if err != nil || !ok {
		t.Fatalf("seen_features missing ok=%v err=%v", ok, err)
	}
	ids := config.ParseSeenIDs(raw)
	if !contains(ids, "ai.discover") {
		t.Fatalf("catalog not marked seen: %v", ids)
	}
}

func TestSaveKeepsTokenWhenEmpty(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "client.yaml")
	if err := os.WriteFile(p, []byte(`version: 1
server:
  url: "https://board.yinger650.com"
  machine_token: "abp_m_keep_me"
machine:
  key: "home-server"
`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SaveAndReload(p, config.Edit{URL: "https://board.example.com"}); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(p)
	if !strings.Contains(string(got), "abp_m_keep_me") {
		t.Fatalf("%s", got)
	}
}

func TestCheckLoopback(t *testing.T) {
	if err := checkLoopback("127.0.0.1:7439"); err != nil {
		t.Fatal(err)
	}
	if err := checkLoopback("0.0.0.0:7439"); err == nil {
		t.Fatal("must reject non-loopback")
	}
}

func TestTUIToggleAndSave(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "client.yaml")
	if err := os.WriteFile(p, []byte(`version: 1
server:
  url: "https://board.yinger650.com"
  machine_token: "abp_m_x"
machine:
  key: "home-server"
storage:
  spool_path: "`+filepath.Join(dir, "spool.db")+`"
collectors:
  cpu: true
`), 0o644); err != nil {
		t.Fatal(err)
	}
	in := strings.NewReader("t cpu\nt ai.discover\ns\nq\n")
	var out strings.Builder
	if err := RunTUI(p, in, &out); err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(p)
	text := string(raw)
	if !strings.Contains(text, "cpu: false") && !strings.Contains(text, "cpu: false\n") {
		if strings.Contains(text, "cpu: true") {
			t.Fatalf("cpu should be toggled off:\n%s", text)
		}
	}
	if !strings.Contains(text, "discover:") || !strings.Contains(text, "enabled: true") {
		t.Fatalf("discover not enabled:\n%s", text)
	}
	if !strings.Contains(out.String(), "AI 主机巡检") {
		t.Fatalf("tui missing catalog:\n%s", out.String())
	}
}

func TestWebSaveTogglesFeature(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "client.yaml")
	if err := os.WriteFile(p, []byte(`version: 1
server:
  url: "https://board.yinger650.com"
  machine_token: "abp_m_x"
machine:
  key: "home-server"
  status_probes:
    - key: gpu
      intent: "util"
storage:
  spool_path: "`+filepath.Join(dir, "spool.db")+`"
collectors:
  cpu: true
`), 0o644); err != nil {
		t.Fatal(err)
	}
	ts := httptest.NewServer(newMux(p))
	defer ts.Close()
	resp, err := http.Get(ts.URL + "/")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := readAll(resp)
	if !strings.Contains(body, "AI 主机巡检") || !strings.Contains(body, `name="feat"`) ||
		!strings.Contains(body, `name="probe_kind"`) || !strings.Contains(body, "CURSOR_API_KEY") ||
		!strings.Contains(body, "<details") || !strings.Contains(body, "Build 并预览") {
		t.Fatalf("page=%s", body)
	}
	form := url.Values{}
	form.Set("url", "https://board.yinger650.com")
	form.Set("key", "home-server")
	form.Add("feat", "cpu")
	form.Add("feat", "ai.discover")
	form.Add("sub.ai.discover", "unit_status")
	form.Add("probe_key", "gpu")
	form.Add("probe_key_edit", "gpu")
	form.Add("probe_kind", "service")
	form.Add("probe_name", "GPU 服务")
	form.Add("probe_intent", "util")
	form.Add("probe_path", "")
	form.Add("probe_interval", "")
	form.Add("probe_ttl", "240")
	form.Add("probe_dir", "")
	form.Add("probe_command", "")
	form.Add("probe_history", "")
	form.Add("probe_extra", "")
	resp, err = http.PostForm(ts.URL+"/save", form)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("status %d %s", resp.StatusCode, mustRead(resp))
	}
	raw, _ := os.ReadFile(p)
	text := string(raw)
	if !strings.Contains(text, "key: gpu") || !strings.Contains(text, "kind: service") ||
		!strings.Contains(text, "name: GPU 服务") || !strings.Contains(text, "ttl_seconds: 240") {
		t.Fatalf("custom probe lost:\n%s", text)
	}
	if !strings.Contains(text, "enabled: true") || !strings.Contains(text, "unit_status") {
		t.Fatalf("discover:\n%s", text)
	}
	if strings.Contains(text, "filesystems:") {
		t.Fatalf("unrelated collector leaked:\n%s", text)
	}
	loaded, err := config.Read(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(loaded.Machine.StatusProbes) != 1 || loaded.Machine.StatusProbes[0].Enabled != nil {
		t.Fatalf("unbuilt existing probe should keep enabled unset: %+v", loaded.Machine.StatusProbes)
	}
}

func TestTUIAddsNaturalLanguageHTTPProbe(t *testing.T) {
	m := &Model{}
	in := strings.NewReader("a\nlocal-health\nhttp\n本机健康\n检查 http://127.0.0.1:18080/health\n\n30s\n180\n")
	var out strings.Builder
	if err := editProbes(m, bufio.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	if len(m.Probes) != 1 {
		t.Fatalf("probes=%+v output=%s", m.Probes, out.String())
	}
	p := m.Probes[0]
	if p.Key != "local-health" || p.Kind != config.StatusProbeHTTP || p.Name != "本机健康" ||
		p.Interval.Duration != 30*time.Second || p.TTLSeconds != 180 {
		t.Fatalf("probe=%+v", p)
	}
	if !strings.Contains(out.String(), "CURSOR_API_KEY") {
		t.Fatalf("missing key hint: %s", out.String())
	}
	if p.IsEnabled() {
		t.Fatal("new probe should start disabled")
	}
}

func TestTUIExpandBuildPreviewEnable(t *testing.T) {
	dir := t.TempDir()
	m := &Model{
		ExtRoot:   filepath.Join(dir, "extensions"),
		LegacyDir: filepath.Join(dir, "probes"),
		Previews:  map[string]string{},
		Provider:  &stubAI{text: "#!/bin/sh\nprintf '%s\\n' '{\"state\":\"running\",\"summary\":\"ok\",\"severity\":\"normal\",\"statuses\":[{\"key\":\"gpu_util\",\"value\":\"41\"}]}'\n"},
	}
	m.Probes = []config.StatusProbe{{
		Key: "gpu", Kind: config.StatusProbeMetric, Intent: "NVIDIA GPU", Enabled: config.BoolPtr(false),
	}}
	in := strings.NewReader("0\nb\ne\n\n\n")
	var out strings.Builder
	if err := editProbes(m, bufio.NewReader(in), &out); err != nil {
		t.Fatal(err)
	}
	text := out.String()
	if !strings.Contains(text, "Build 完成") || !strings.Contains(text, "gpu_util") {
		t.Fatalf("preview missing:\n%s", text)
	}
	if !m.probeBuilt(m.Probes[0]) {
		t.Fatal("expected artifact")
	}
	if !m.Probes[0].IsEnabled() {
		t.Fatal("enable after build")
	}
	if m.Probes[0].Dir != "nl/gpu" {
		t.Fatalf("dir=%s", m.Probes[0].Dir)
	}
}

func TestWebBuildWritesPreview(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "client.yaml")
	if err := os.WriteFile(p, []byte(`version: 1
server:
  url: "https://board.yinger650.com"
  machine_token: "abp_m_x"
machine:
  key: "home-server"
  status_probes:
    - key: gpu
      intent: "util"
storage:
  spool_path: "`+filepath.Join(dir, "spool.db")+`"
ai:
  enabled: true
`), 0o644); err != nil {
		t.Fatal(err)
	}
	script := "#!/bin/sh\nprintf '%s\\n' '{\"state\":\"running\",\"summary\":\"ok\",\"severity\":\"normal\",\"statuses\":[{\"key\":\"gpu_util\",\"value\":\"41\"}]}'\n"
	ts := httptest.NewServer(newWeb(p, &stubAI{text: script}))
	defer ts.Close()
	form := url.Values{}
	form.Set("url", "https://board.yinger650.com")
	form.Set("key", "home-server")
	form.Add("feat", "cpu")
	form.Add("probe_key", "gpu")
	form.Add("probe_key_edit", "gpu")
	form.Add("probe_kind", "metric")
	form.Add("probe_name", "gpu")
	form.Add("probe_intent", "util")
	form.Add("probe_path", "")
	form.Add("probe_interval", "")
	form.Add("probe_ttl", "")
	form.Add("probe_dir", "")
	form.Add("probe_command", "")
	form.Add("probe_history", "")
	form.Add("probe_extra", "")
	form.Set("probe_index", "0")
	resp, err := http.PostForm(ts.URL+"/build", form)
	if err != nil {
		t.Fatal(err)
	}
	body := mustRead(resp)
	if resp.StatusCode != 200 {
		t.Fatalf("status %d %s", resp.StatusCode, body)
	}
	if !strings.Contains(body, "Build 完成") || !strings.Contains(body, "gpu_util") {
		t.Fatalf("page=%s", body)
	}
	if _, err := os.Stat(filepath.Join(dir, "extensions", "nl", "gpu", "probe.sh")); err != nil {
		t.Fatal(err)
	}
	form.Add("probe_enable", "gpu")
	form.Add("probe_dir", "nl/gpu")
	resp, err = http.PostForm(ts.URL+"/save", form)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("save %d %s", resp.StatusCode, mustRead(resp))
	}
	raw, _ := os.ReadFile(p)
	text := string(raw)
	if !strings.Contains(text, "dir: nl/gpu") || !strings.Contains(text, "intent: util") {
		t.Fatalf("yaml:\n%s", text)
	}
}

type stubAI struct{ text string }

func (s *stubAI) Name() string { return "stub" }
func (s *stubAI) Run(_ context.Context, _ aiprovider.Request) (aiprovider.Result, error) {
	return aiprovider.Result{Text: s.text}, nil
}

func readAll(resp *http.Response) (string, error) {
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	return string(b), err
}

func mustRead(resp *http.Response) string {
	s, _ := readAll(resp)
	return s
}

func contains(ids []string, want string) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}
