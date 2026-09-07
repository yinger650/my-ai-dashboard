package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestOverlayEnablesDiscoverWithoutDefaultsPollution(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "client.yaml")
	src := `# keep this comment
version: 1
server:
  url: "https://board.yinger650.com"
  machine_token: "abp_m_keep"
machine:
  key: "home-server"
  status_probes:
    - key: gpu
      intent: "util"
collectors:
  cpu: true
  http:
    enabled: false
    targets:
      - service_key: site-board
        name: AgentBoard
        url: "https://board.yinger650.com/health/live"
`
	if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	err := ApplyEdit(p, Edit{
		Features: map[string]bool{
			"cpu":                     true,
			"ai.discover":             true,
			"ai.summarize.agent_logs": false,
		},
		Subs: map[string]map[string]bool{
			"ai.discover": {
				"unit_status":  true,
				"unit_journal": true,
				"read_file":    false,
			},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	text := string(raw)
	if !strings.Contains(text, "keep this comment") {
		t.Fatalf("comment lost:\n%s", text)
	}
	if strings.Contains(text, "systemd") && strings.Contains(text, "include:") {
		t.Fatalf("applyDefaults systemd leaked:\n%s", text)
	}
	if !strings.Contains(text, "abp_m_keep") {
		t.Fatal("token overwritten")
	}
	if !strings.Contains(text, "site-board") || !strings.Contains(text, "key: gpu") {
		t.Fatalf("custom lists lost:\n%s", text)
	}
	doc, cfg, _, err := LoadDocument(p)
	if err != nil {
		t.Fatal(err)
	}
	root := rootMap(doc)
	f, _ := FeatureByID("ai.discover")
	if !FeatureEnabled(root, f) {
		t.Fatal("discover not enabled")
	}
	if !SubEnabled(root, "ai.discover", "unit_status") || SubEnabled(root, "ai.discover", "read_file") {
		t.Fatalf("subs=%v", cfg.AI.Discover.AllowCommands)
	}
	if cfg.AI.Enabled {
		// unmarshal without applyDefaults: we set ai.enabled true via dependency
	}
	ai, _ := FeatureByID("ai")
	if !FeatureEnabled(root, ai) {
		t.Fatal("ai.enabled should be forced on")
	}
}

func TestOverlayKeepsTokenWhenEmpty(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "client.yaml")
	src := `version: 1
server:
  url: "https://board.yinger650.com"
  machine_token: "abp_m_original"
machine:
  key: "home-server"
`
	if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := ApplyEdit(p, Edit{URL: "https://board.yinger650.com", Token: ""}); err != nil {
		t.Fatal(err)
	}
	got, _ := os.ReadFile(p)
	if !strings.Contains(string(got), "abp_m_original") {
		t.Fatalf("%s", got)
	}
}

func TestOverlayNewFileSkeleton(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "missing.yaml")
	err := ApplyEdit(p, Edit{
		URL:        "https://board.yinger650.com",
		MachineKey: "home-server",
		Features: map[string]bool{
			"cpu":    true,
			"memory": true,
			"http":   false,
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := os.ReadFile(p)
	text := string(raw)
	if !strings.Contains(text, "machine_token_env") || !strings.Contains(text, "spool_path") {
		t.Fatalf("skeleton missing:\n%s", text)
	}
	if strings.Contains(text, "board-server.service") {
		t.Fatalf("systemd include seeded:\n%s", text)
	}
}

func TestSeedOnlyWhenSubtreeEmpty(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "client.yaml")
	src := `version: 1
server:
  url: "http://127.0.0.1:1"
machine:
  key: "x"
ai:
  enabled: false
  summarize:
    - source: probe:gpu
      service_key: gpu-digest
      name: GPU
  discover:
    enabled: false
    allow_commands:
      - id: custom_ps
        argv: ["ps", "aux"]
`
	if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	err := ApplyEdit(p, Edit{
		Features: map[string]bool{
			"ai.discover":             true,
			"ai.summarize.agent_logs": true,
		},
		Subs: map[string]map[string]bool{
			"ai.discover": {"unit_status": true, "unit_journal": false, "read_file": false},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	_, cfg, _, err := LoadDocument(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.AI.Summarize) != 2 {
		t.Fatalf("summarize=%v", cfg.AI.Summarize)
	}
	var sources []string
	for _, s := range cfg.AI.Summarize {
		sources = append(sources, s.Source)
	}
	if !containsStr(sources, "probe:gpu") || !containsStr(sources, "agent_logs") {
		t.Fatalf("sources=%v", sources)
	}
	ids := map[string]bool{}
	for _, c := range cfg.AI.Discover.AllowCommands {
		ids[c.ID] = true
	}
	if !ids["custom_ps"] || !ids["unit_status"] {
		t.Fatalf("allow=%v", cfg.AI.Discover.AllowCommands)
	}
	if ids["unit_journal"] {
		t.Fatal("unchecked default should be omitted")
	}
}
