package config

import (
	"os"
	"path/filepath"
	"testing"
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
}

func TestValidateRejectsBadKeyAndMissingToken(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "client.yaml")
	_ = os.WriteFile(p, []byte("version: 1\nserver:\n  url: \"http://x\"\n  machine_token_env: \"MISSING_VAR\"\nmachine:\n  key: \"BAD KEY\"\n"), 0o644)
	if _, err := Load(p); err == nil {
		t.Fatal("expected validation error for bad key / missing token")
	}
}
