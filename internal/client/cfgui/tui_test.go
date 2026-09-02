package cfgui

import (
	"path/filepath"
	"strings"
	"testing"

	"agentboard/internal/client/config"
)

func TestSaveWritesYAML(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "client.yaml")
	c := &config.Config{}
	c.Version = 1
	c.Server.URL = "https://board.yinger650.com"
	c.Server.MachineToken = "abp_m_ui_token_value"
	c.Machine.Key = "home-server"
	c.Machine.StatusProbes = []config.StatusProbe{{Key: "gpu", Intent: "util"}}
	c.Storage.SpoolPath = filepath.Join(dir, "spool.db")
	if err := SaveAndReload(p, c); err != nil {
		t.Fatal(err)
	}
	got, err := config.Read(p)
	if err != nil {
		t.Fatal(err)
	}
	if got.Server.URL != c.Server.URL || got.Machine.StatusProbes[0].Key != "gpu" {
		t.Fatalf("%+v", got)
	}
	if !strings.Contains(got.Server.MachineToken, "abp_m_ui_token_value") {
		t.Fatalf("token not saved")
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
