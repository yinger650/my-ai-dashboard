package server

import (
	"encoding/json"
	"net/http"
	"net/http/cookiejar"
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
