package server

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
	"strings"
	"testing"
	"time"

	"agentboard/internal/event"
	"agentboard/internal/shared"
	"agentboard/internal/store"
)

func TestSettingsExposeMonthAndFiveGigQuota(t *testing.T) {
	srv, _ := newTestServer(t)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: 10 * time.Second}
	code, env := doJSON(t, client, http.MethodPost, srv.URL+"/auth/login", "", map[string]string{"password": "super-secret-password"})
	if code != 200 {
		t.Fatalf("login %d", code)
	}
	code, env = doJSON(t, client, http.MethodGet, srv.URL+"/api/v1/admin/settings", "", nil)
	if code != 200 {
		t.Fatalf("settings %d %s", code, env.Data)
	}
	var got map[string]any
	if err := json.Unmarshal(env.Data, &got); err != nil {
		t.Fatal(err)
	}
	if got["event_retention_days"] != float64(30) {
		t.Fatalf("event days %+v", got["event_retention_days"])
	}
	if got["access_retention_days"] != float64(30) {
		t.Fatalf("access days %+v", got["access_retention_days"])
	}
	if got["event_quota_bytes"] != float64(5*1024*1024*1024) {
		t.Fatalf("quota %+v", got["event_quota_bytes"])
	}
}

func TestMaintenanceRunOK(t *testing.T) {
	srv, _ := newTestServer(t)
	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: 10 * time.Second}
	code, env := doJSON(t, client, http.MethodPost, srv.URL+"/auth/login", "", map[string]string{"password": "super-secret-password"})
	if code != 200 {
		t.Fatalf("login %d", code)
	}
	code, env = doJSON(t, client, http.MethodGet, srv.URL+"/auth/session", "", nil)
	if code != 200 {
		t.Fatalf("session %d", code)
	}
	var sess struct {
		CSRFToken string `json:"csrf_token"`
	}
	_ = json.Unmarshal(env.Data, &sess)
	code, env = doJSON(t, client, http.MethodPost, srv.URL+"/api/v1/admin/maintenance/run", sess.CSRFToken, map[string]any{})
	if code != 200 {
		t.Fatalf("maintenance %d %s", code, env.Data)
	}
	if !json.Valid(env.Data) {
		t.Fatalf("invalid json %s", env.Data)
	}
}

func TestMaintenanceClosesStaleRuns(t *testing.T) {
	srv, st := newTestServer(t)
	ctx := t.Context()
	m := &store.Machine{MachineKey: "stale-web", Name: "Stale Web", Kind: "virtual", Enabled: true, AutoCreateServices: true}
	if err := st.CreateMachine(ctx, m); err != nil {
		t.Fatal(err)
	}
	auth := store.IngestAuth{MachineID: m.ID, AutoCreateServices: true}
	now := shared.NowUTC()
	received := shared.FormatTime(now)
	pb, _ := json.Marshal(event.RunTransition{ServiceName: "Cursor", ServiceType: "agent", Status: "running", Summary: "abandoned"})
	runEnv := &event.Envelope{
		SchemaVersion: 1,
		EventID:       shared.NewID(),
		EventType:     event.TypeRunTransition,
		OccurredAt:    shared.FormatTime(now.Add(-48 * time.Hour)),
		ServiceKey:    "cursor",
		RunKey:        "abandoned-1",
		Payload:       pb,
	}
	if r, err := st.IngestEvent(ctx, runEnv, auth, received); err != nil || r.Status != "accepted" {
		t.Fatalf("run: %v %+v", err, r)
	}
	svc, err := st.GetServiceByKey(ctx, m.ID, "cursor")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB().ExecContext(ctx, `UPDATE runs SET created_at = ? WHERE service_id = ? AND run_key = ?`,
		shared.FormatTime(now.Add(-48*time.Hour)), svc.ID, "abandoned-1"); err != nil {
		t.Fatal(err)
	}

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: 10 * time.Second}
	code, env := doJSON(t, client, http.MethodPost, srv.URL+"/auth/login", "", map[string]string{"password": "super-secret-password"})
	if code != 200 {
		t.Fatalf("login %d", code)
	}
	code, env = doJSON(t, client, http.MethodGet, srv.URL+"/auth/session", "", nil)
	if code != 200 {
		t.Fatalf("session %d", code)
	}
	var sess struct {
		CSRFToken string `json:"csrf_token"`
	}
	_ = json.Unmarshal(env.Data, &sess)
	code, env = doJSON(t, client, http.MethodPost, srv.URL+"/api/v1/admin/maintenance/run", sess.CSRFToken, map[string]any{})
	if code != 200 {
		t.Fatalf("maintenance %d %s", code, env.Data)
	}
	var got map[string]any
	if err := json.Unmarshal(env.Data, &got); err != nil {
		t.Fatal(err)
	}
	if got["runs_closed"] != float64(1) {
		t.Fatalf("runs_closed %+v", got)
	}
	runs, err := st.ListRuns(ctx, svc.ID, 10)
	if err != nil || len(runs) != 1 || runs[0].Status != "timed_out" {
		t.Fatalf("run after close: %v %+v", err, runs)
	}
}

func TestBoardGETClosesStaleRuns(t *testing.T) {
	srv, st := newTestServer(t)
	ctx := t.Context()
	m := &store.Machine{MachineKey: "board-stale", Name: "Board Stale", Kind: "virtual", Enabled: true, AutoCreateServices: true}
	if err := st.CreateMachine(ctx, m); err != nil {
		t.Fatal(err)
	}
	auth := store.IngestAuth{MachineID: m.ID, AutoCreateServices: true}
	now := shared.NowUTC()
	received := shared.FormatTime(now)
	ingestRun := func(key, status, summary string) {
		t.Helper()
		pb, _ := json.Marshal(event.RunTransition{ServiceName: "Cursor", ServiceType: "agent", Status: status, Summary: summary})
		env := &event.Envelope{
			SchemaVersion: 1,
			EventID:       shared.NewID(),
			EventType:     event.TypeRunTransition,
			OccurredAt:    received,
			ServiceKey:    "cursor",
			RunKey:        key,
			Payload:       pb,
		}
		if r, err := st.IngestEvent(ctx, env, auth, received); err != nil || r.Status != "accepted" {
			t.Fatalf("run %s: %v %+v", key, err, r)
		}
	}
	ingestRun("abandoned-1", "running", "old task")
	ingestRun("live-1", "running", "fresh task")
	svc, err := st.GetServiceByKey(ctx, m.ID, "cursor")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := st.DB().ExecContext(ctx, `UPDATE runs SET created_at = ? WHERE service_id = ? AND run_key = ?`,
		shared.FormatTime(now.Add(-48*time.Hour)), svc.ID, "abandoned-1"); err != nil {
		t.Fatal(err)
	}

	jar, _ := cookiejar.New(nil)
	client := &http.Client{Jar: jar, Timeout: 10 * time.Second}
	code, env := doJSON(t, client, http.MethodPost, srv.URL+"/auth/login", "", map[string]string{"password": "super-secret-password"})
	if code != 200 {
		t.Fatalf("login %d", code)
	}
	code, env = doJSON(t, client, http.MethodGet, srv.URL+"/api/v1/board", "", nil)
	if code != 200 {
		t.Fatalf("board %d %s", code, env.Data)
	}
	var board struct {
		Machines []struct {
			ID         string `json:"id"`
			ActiveRuns []struct {
				RunKey  string `json:"run_key"`
				Status  string `json:"status"`
				Summary string `json:"summary"`
			} `json:"active_runs"`
			Services []struct {
				ServiceKey   string `json:"service_key"`
				RunningCount int    `json:"running_count"`
				StateSummary string `json:"state_summary"`
			} `json:"services"`
		} `json:"machines"`
	}
	if err := json.Unmarshal(env.Data, &board); err != nil {
		t.Fatal(err)
	}
	idx := -1
	for i := range board.Machines {
		if board.Machines[i].ID == m.ID {
			idx = i
			break
		}
	}
	if idx < 0 {
		t.Fatalf("machine missing: %s", env.Data)
	}
	card := board.Machines[idx]
	if len(card.ActiveRuns) != 1 || card.ActiveRuns[0].RunKey != "live-1" {
		t.Fatalf("active_runs=%+v want only live-1", card.ActiveRuns)
	}
	var runningCount int
	var summary string
	for _, svcRow := range card.Services {
		if svcRow.ServiceKey == "cursor" {
			runningCount = svcRow.RunningCount
			summary = svcRow.StateSummary
		}
	}
	if runningCount != 1 {
		t.Fatalf("running_count=%d want 1 summary=%q", runningCount, summary)
	}
	if !strings.Contains(summary, "1 进行中") {
		t.Fatalf("state_summary=%q", summary)
	}

	runs, err := st.ListRuns(ctx, svc.ID, 10)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, r := range runs {
		got[r.RunKey] = r.Status
	}
	if got["abandoned-1"] != "timed_out" || got["live-1"] != "running" {
		t.Fatalf("statuses=%v", got)
	}
}
