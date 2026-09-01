package probe

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agentboard/internal/event"
)

func TestCheckScriptRejectsRelativeAndWritable(t *testing.T) {
	if err := CheckScript("gpu.sh"); err == nil {
		t.Fatal("relative path")
	}
	dir := t.TempDir()
	p := filepath.Join(dir, "ok.sh")
	if err := os.WriteFile(p, []byte("#!/bin/sh\necho hi\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := CheckScript(p); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(p, 0o666); err != nil {
		t.Fatal(err)
	}
	if err := CheckScript(p); err == nil {
		t.Fatal("writable should be rejected")
	}
}

func TestExpandUnitAndPath(t *testing.T) {
	dir := t.TempDir()
	f := filepath.Join(dir, "a.log")
	if err := os.WriteFile(f, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	out, err := Expand([]string{"journalctl", "-u", "{unit}"}, map[string]string{"unit": "sshd.service"}, nil)
	if err != nil || out[2] != "sshd.service" {
		t.Fatalf("%v %v", out, err)
	}
	if _, err := Expand([]string{"journalctl", "-u", "{unit}"}, map[string]string{"unit": "sshd; rm -rf /"}, nil); err == nil {
		t.Fatal("bad unit")
	}
	if _, err := Expand([]string{"cat", "{path}"}, map[string]string{"path": f}, []string{dir + "/**"}); err != nil {
		t.Fatal(err)
	}
	if _, err := Expand([]string{"cat", "{path}"}, map[string]string{"path": "/etc/passwd"}, []string{dir + "/**"}); err == nil {
		t.Fatal("path outside allow_paths")
	}
	if _, err := Expand([]string{"cat", "{path}"}, map[string]string{"path": "rel.log"}, []string{dir + "/**"}); err == nil {
		t.Fatal("relative path")
	}
}

func TestMapJSONOwnsServiceKeyOnly(t *testing.T) {
	raw := []byte(`{"state":"running","summary":"4 卡在跑","severity":"normal",
		"statuses":[{"key":"gpu_util","label":"GPU 利用率","value":"87","unit":"%","severity":"warning"}],
		"logs":[{"markdown":"OOM on card 3","severity":"error"}],
		"pinned_markdown":"| 卡 | 显存 |\n|---|---|\n| 0 | 71G |",
		"event_type":"machine.heartbeat","service_key":"other"}`)
	r, err := ParseJSON(raw)
	if err != nil {
		t.Fatal(err)
	}
	evs, hash := MapJSON("gpu", "GPU 节点", 180, r, "")
	if hash == "" {
		t.Fatal("expected pin hash")
	}
	keys := map[string]int{}
	types := map[string]int{}
	for _, e := range evs {
		keys[e.ServiceKey]++
		types[e.Type]++
		if e.ServiceKey != "gpu" {
			t.Fatalf("leaked service_key %q", e.ServiceKey)
		}
	}
	if types[event.TypeServiceState] != 1 || types[event.TypeStatusUpsert] != 1 || types[event.TypeLogAppend] != 1 || types[event.TypeLogPin] != 1 {
		t.Fatalf("types %v", types)
	}
	evs2, hash2 := MapJSON("gpu", "GPU 节点", 180, r, hash)
	if hash2 != hash {
		t.Fatal("hash should be stable")
	}
	for _, e := range evs2 {
		if e.Type == event.TypeLogPin {
			t.Fatal("unchanged pin should be skipped")
		}
	}
}

func TestRunScriptEnvAndTimeout(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "env.sh")
	src := "#!/bin/sh\nif [ -n \"$ABP_MACHINE_TOKEN\" ]; then echo TOKEN_LEAK; exit 1; fi\necho '{\"state\":\"running\",\"summary\":\"ok\",\"severity\":\"normal\"}'\n"
	if err := os.WriteFile(p, []byte(src), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("ABP_MACHINE_TOKEN", "abp_m_should_not_leak")
	out, trunc, err := RunScript(context.Background(), []string{p}, 2*time.Second, 4096)
	if err != nil || trunc {
		t.Fatalf("err=%v trunc=%v out=%s", err, trunc, out)
	}
	if strings.Contains(string(out), "TOKEN_LEAK") {
		t.Fatal("token leaked into probe env")
	}
	var r Result
	if err := json.Unmarshal(out, &r); err != nil || r.State != "running" {
		t.Fatalf("json %v %+v", err, r)
	}

	slow := filepath.Join(dir, "slow.sh")
	if err := os.WriteFile(slow, []byte("#!/bin/sh\nsleep 5\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	_, _, err = RunScript(context.Background(), []string{slow}, 200*time.Millisecond, 1024)
	if err == nil {
		t.Fatal("expected timeout")
	}
}
