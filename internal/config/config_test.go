package config

import (
	"strings"
	"testing"
)

func TestLoadRetentionDefaults(t *testing.T) {
	t.Setenv("ABP_DATA_DIR", t.TempDir())
	t.Setenv("ABP_EVENT_RETENTION_DAYS", "")
	t.Setenv("ABP_ACCESS_RETENTION_DAYS", "")
	t.Setenv("ABP_EVENT_QUOTA_BYTES", "")
	t.Setenv("ABP_PUBLIC_URL", "http://127.0.0.1")
	c, err := Load()
	if err != nil {
		t.Fatal(err)
	}
	if c.EventRetention != 30 || c.AccessRetention != 30 || c.RawMetricRetention != 30 {
		t.Fatalf("days = event %d access %d metric %d", c.EventRetention, c.AccessRetention, c.RawMetricRetention)
	}
	if c.EventQuotaBytes != 5*1024*1024*1024 {
		t.Fatalf("quota = %d", c.EventQuotaBytes)
	}
	if !c.ClientUpdateSync {
		t.Fatal("client update sync should default on")
	}
	if !strings.HasSuffix(c.ClientUpdateDir, "client-updates") {
		t.Fatalf("client update dir = %q", c.ClientUpdateDir)
	}
}
