package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const sample = `version: 1
server:
  url: "http://127.0.0.1:8080"
  machine_token_env: "TEST_TOKEN_VAR"
machine:
  key: "home-server"
intervals:
  heartbeat: 15s
  metrics: 45s
collectors:
  cpu: true
`

func TestLoadAndValidate(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "client.yaml")
	if err := os.WriteFile(p, []byte(sample), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TEST_TOKEN_VAR", "abp_m_secret")

	c, err := Load(p)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if c.Machine.Key != "home-server" {
		t.Errorf("key = %q", c.Machine.Key)
	}
	if c.Intervals.Heartbeat.Duration.Seconds() != 15 {
		t.Errorf("heartbeat = %v", c.Intervals.Heartbeat.Duration)
	}
	if c.Intervals.Metrics.Duration.Seconds() != 45 {
		t.Errorf("metrics = %v", c.Intervals.Metrics.Duration)
	}
	// defaults applied
	if c.Storage.MaxEvents != 50000 {
		t.Errorf("default max_events = %d", c.Storage.MaxEvents)
	}
	if c.Token() != "abp_m_secret" {
		t.Errorf("token = %q", c.Token())
	}
	if !c.LocalIngestOn() {
		t.Fatal("local ingest should default on")
	}
	if c.LocalIngest.Listen != "127.0.0.1:7438" {
		t.Errorf("listen = %q", c.LocalIngest.Listen)
	}
	if !strings.HasSuffix(c.LocalIngest.AdvertisePath, "local-ingest.json") {
		t.Errorf("advertise = %q", c.LocalIngest.AdvertisePath)
	}
	if c.Update.Enabled {
		t.Fatal("update should default off")
	}
	if c.Update.URL != "https://github.com/yinger650/my-ai-dashboard/releases/latest/download" {
		t.Errorf("update url = %q", c.Update.URL)
	}
	if c.Update.Interval.Duration != time.Hour {
		t.Errorf("update interval = %v", c.Update.Interval.Duration)
	}
}

func TestLoadHTTPTargets(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "client.yaml")
	src := `version: 1
server:
  url: "https://board.yinger650.com"
  machine_token_env: "TEST_TOKEN_VAR"
machine:
  key: "aliyun-web"
collectors:
  http:
    enabled: true
    targets:
      - url: "https://board.yinger650.com/health/live"
        expect_status: [200]
      - service_key: site-custom
        name: Custom
        url: "https://example.com/"
`
	if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Setenv("TEST_TOKEN_VAR", "abp_m_secret")
	c, err := Load(p)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if !c.Collectors.HTTP.Enabled {
		t.Fatal("http collector should be enabled")
	}
	if c.Intervals.HTTP.Duration != time.Minute {
		t.Errorf("http interval default = %v", c.Intervals.HTTP.Duration)
	}
	if len(c.Collectors.HTTP.Targets) != 2 {
		t.Fatalf("targets = %d", len(c.Collectors.HTTP.Targets))
	}
	if c.Collectors.HTTP.Targets[0].ServiceKey != "site-board-yinger650-com" {
		t.Errorf("auto service_key = %q", c.Collectors.HTTP.Targets[0].ServiceKey)
	}
	if c.Collectors.HTTP.Targets[0].Name != "board.yinger650.com" {
		t.Errorf("auto name = %q", c.Collectors.HTTP.Targets[0].Name)
	}
	if c.Collectors.HTTP.Targets[1].ServiceKey != "site-custom" {
		t.Errorf("explicit key = %q", c.Collectors.HTTP.Targets[1].ServiceKey)
	}
	if !c.HTTPFollowRedirects() {
		t.Fatal("follow_redirects should default true")
	}
}

func TestHTTPRejectsBadURLAndDuplicateKey(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "client.yaml")
	t.Setenv("TEST_TOKEN_VAR", "abp_m_secret")
	src := `version: 1
server:
  url: "https://board.yinger650.com"
  machine_token_env: "TEST_TOKEN_VAR"
machine:
  key: "aliyun-web"
collectors:
  http:
    enabled: true
    targets:
      - service_key: site-a
        url: "ftp://example.com"
`
	if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(p); err == nil {
		t.Fatal("expected invalid URL error")
	}

	src = `version: 1
server:
  url: "https://board.yinger650.com"
  machine_token_env: "TEST_TOKEN_VAR"
machine:
  key: "aliyun-web"
collectors:
  http:
    enabled: true
    targets:
      - service_key: site-a
        url: "https://a.example/"
      - service_key: site-a
        url: "https://b.example/"
`
	if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(p); err == nil {
		t.Fatal("expected duplicate service_key error")
	}
}

func TestAIAndProbeValidation(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "client.yaml")
	t.Setenv("TEST_TOKEN_VAR", "abp_m_secret")
	src := `version: 1
server:
  url: "http://127.0.0.1:8080"
  machine_token_env: "TEST_TOKEN_VAR"
machine:
  key: "dev"
ai:
  enabled: true
  provider: command
  command: ["/bin/true"]
  summarize:
    - source: agent_logs
      service_key: ai-agent-digest
collectors:
  probes:
    enabled: true
    scripts:
      - service_key: load
        command: ["/usr/local/bin/load.sh"]
        format: json
`
	if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := Load(p)
	if err != nil {
		t.Fatal(err)
	}
	if c.AI.Timeout.Duration != 120*time.Second {
		t.Errorf("timeout default %v", c.AI.Timeout.Duration)
	}
	if !c.AI.FallbackHeuristicOn() {
		t.Fatal("fallback default")
	}
	if c.Collectors.Probes.Scripts[0].Timeout.Duration != 15*time.Second {
		t.Fatal("probe timeout default")
	}

	bad := strings.Replace(src, "command: [\"/usr/local/bin/load.sh\"]", "command: [\"load.sh\"]", 1)
	if err := os.WriteFile(p, []byte(bad), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(p); err == nil {
		t.Fatal("relative probe path should fail")
	}
}

func TestUpdateRejectsBadURL(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "client.yaml")
	t.Setenv("TEST_TOKEN_VAR", "abp_m_secret")
	src := `version: 1
server:
  url: "http://127.0.0.1:8080"
  machine_token_env: "TEST_TOKEN_VAR"
machine:
  key: "dev"
update:
  enabled: true
  url: "not-a-url"
`
	if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Load(p); err == nil {
		t.Fatal("expected invalid update.url")
	}
}

func TestValidateRejectsBadKeyAndMissingToken(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "client.yaml")
	_ = os.WriteFile(p, []byte("version: 1\nserver:\n  url: \"http://x\"\n  machine_token_env: \"MISSING_VAR\"\nmachine:\n  key: \"BAD KEY\"\n"), 0o644)
	if _, err := Load(p); err == nil {
		t.Fatal("expected validation error for bad key / missing token")
	}
}
