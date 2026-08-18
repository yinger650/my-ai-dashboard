package store

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"testing"

	"agentboard/internal/event"
	"agentboard/internal/shared"
)

func TestReportScriptDryRunAndLive(t *testing.T) {
	script := filepath.Join("..", "..", "skills", "agentboard-report", "scripts", "report.py")
	if _, err := os.Stat(script); err != nil {
		script = filepath.Join("..", "..", "..", "skills", "agentboard-report", "scripts", "report.py")
	}
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	script = filepath.Join(root, "skills", "agentboard-report", "scripts", "report.py")
	if _, err := os.Stat(script); err != nil {
		t.Skip("report.py not found")
	}

	cmd := exec.Command("python3", script, "--dry-run", "start", "hello")
	cmd.Env = append(os.Environ(),
		"AGENTBOARD_PROVIDER=openclaw",
		"AGENTBOARD_SERVICE_KEY=openclaw",
		"AGENTBOARD_TOKEN=abp_m_test",
	)
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("dry-run: %v\n%s", err, out)
	}
	var body struct {
		Events []map[string]any `json:"events"`
	}
	if err := json.Unmarshal(out, &body); err != nil {
		t.Fatalf("json: %v\n%s", err, out)
	}
	if len(body.Events) < 2 {
		t.Fatalf("want start events, got %s", out)
	}

	var got []json.RawMessage
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer abp_m_test" {
			t.Errorf("auth header")
		}
		var req struct {
			Events []json.RawMessage `json:"events"`
		}
		_ = json.NewDecoder(r.Body).Decode(&req)
		got = req.Events
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"data":{"accepted":1,"duplicates":0,"rejected":0,"results":[]}}`))
	}))
	defer srv.Close()

	cmd = exec.Command("python3", script, "heartbeat", "alive")
	cmd.Env = append(os.Environ(),
		"AGENTBOARD_URL="+srv.URL,
		"AGENTBOARD_PROVIDER=openclaw",
		"AGENTBOARD_SERVICE_KEY=openclaw",
		"AGENTBOARD_TOKEN=abp_m_test",
		"AGENTBOARD_SOFT_FAIL=0",
	)
	if out, err = cmd.CombinedOutput(); err != nil {
		t.Fatalf("heartbeat: %v\n%s", err, out)
	}
	if len(got) == 0 {
		t.Fatal("server received no events")
	}

	st := newTestStore(t)
	ctx := context.Background()
	m := &Machine{MachineKey: "agents", Name: "Agents", Kind: "virtual", Enabled: true, AutoCreateServices: true}
	if err := st.CreateMachine(ctx, m); err != nil {
		t.Fatal(err)
	}
	auth := IngestAuth{MachineID: m.ID, AutoCreateServices: true}
	now := shared.FormatTime(shared.NowUTC())
	for i, raw := range body.Events {
		b, err := json.Marshal(raw)
		if err != nil {
			t.Fatal(err)
		}
		var env event.Envelope
		if err := json.Unmarshal(b, &env); err != nil {
			t.Fatalf("envelope %d: %v", i, err)
		}
		res, err := st.IngestEvent(ctx, &env, auth, now)
		if err != nil || res.Status != "accepted" {
			t.Fatalf("ingest start event %d: err=%v res=%+v payload=%s", i, err, res, b)
		}
	}
	svc, err := st.GetServiceByKey(ctx, m.ID, "openclaw")
	if err != nil {
		t.Fatal(err)
	}
	if svc.Type != "agent" || svc.CurrentState != "running" {
		t.Fatalf("service after start: %+v", svc)
	}
	if svc.TTLSeconds == nil || *svc.TTLSeconds != 180 {
		t.Fatalf("ttl: %+v", svc.TTLSeconds)
	}
}
