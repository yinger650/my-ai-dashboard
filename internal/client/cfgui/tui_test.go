package cfgui

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"

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
	if !strings.Contains(body, "AI 主机巡检") || !strings.Contains(body, `name="feat"`) {
		t.Fatalf("page=%s", body)
	}
	form := url.Values{}
	form.Set("url", "https://board.yinger650.com")
	form.Set("key", "home-server")
	form.Add("feat", "cpu")
	form.Add("feat", "ai.discover")
	form.Add("sub.ai.discover", "unit_status")
	form.Add("probe_key", "gpu")
	form.Add("probe_intent", "util")
	form.Add("probe_path", "")
	form.Add("probe_interval", "")
	resp, err = http.PostForm(ts.URL+"/save", form)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("status %d %s", resp.StatusCode, mustRead(resp))
	}
	raw, _ := os.ReadFile(p)
	text := string(raw)
	if !strings.Contains(text, "key: gpu") {
		t.Fatalf("custom probe lost:\n%s", text)
	}
	if !strings.Contains(text, "enabled: true") || !strings.Contains(text, "unit_status") {
		t.Fatalf("discover:\n%s", text)
	}
	if strings.Contains(text, "filesystems:") {
		t.Fatalf("unrelated collector leaked:\n%s", text)
	}
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
