package store

import (
	"strings"
	"testing"
	"time"

	"agentboard/internal/event"
	"agentboard/internal/shared"
)

func TestApplyRetentionDeletesOldEventsKeepsPin(t *testing.T) {
	ctx := t.Context()
	st := newTestStore(t)
	m := &Machine{MachineKey: "retbox", Name: "Ret", Kind: "vm", Enabled: true, AutoCreateServices: true}
	if err := st.CreateMachine(ctx, m); err != nil {
		t.Fatal(err)
	}
	auth := IngestAuth{MachineID: m.ID, AutoCreateServices: true}
	received := shared.FormatTime(shared.NowUTC())

	if r, err := st.IngestEvent(ctx, mkEnv(t, event.TypeServiceState, "app", "", event.ServiceState{
		Name: "App", Type: "daemon", State: "running", Severity: "normal",
	}), auth, received); err != nil || r.Status != "accepted" {
		t.Fatalf("state: %v %+v", err, r)
	}

	old := mkEnv(t, event.TypeLogAppend, "app", "", event.LogPayload{Markdown: "old log", Severity: "info", Source: "test"})
	old.OccurredAt = shared.FormatTime(shared.NowUTC().Add(-40 * 24 * time.Hour))
	if r, err := st.IngestEvent(ctx, old, auth, received); err != nil || r.Status != "accepted" {
		t.Fatalf("old log: %v %+v", err, r)
	}
	fresh := mkEnv(t, event.TypeLogAppend, "app", "", event.LogPayload{Markdown: "fresh log", Severity: "info", Source: "test"})
	if r, err := st.IngestEvent(ctx, fresh, auth, received); err != nil || r.Status != "accepted" {
		t.Fatalf("fresh log: %v %+v", err, r)
	}
	pin := mkEnv(t, event.TypeLogPin, "app", "", event.LogPayload{Markdown: "pinned", Severity: "info"})
	pin.OccurredAt = shared.FormatTime(shared.NowUTC().Add(-40 * 24 * time.Hour))
	if r, err := st.IngestEvent(ctx, pin, auth, received); err != nil || r.Status != "accepted" {
		t.Fatalf("pin: %v %+v", err, r)
	}

	oldAccess := &AccessLog{
		RequestID: "r1", ActorType: "admin", Method: "GET", Path: "/api/v1/board",
		StatusCode: 200, Result: "ok",
		OccurredAt: shared.FormatTime(shared.NowUTC().Add(-40 * 24 * time.Hour)),
	}
	if err := st.InsertAccessLog(ctx, oldAccess); err != nil {
		t.Fatal(err)
	}
	freshAccess := &AccessLog{
		RequestID: "r2", ActorType: "admin", Method: "GET", Path: "/api/v1/board",
		StatusCode: 200, Result: "ok",
	}
	if err := st.InsertAccessLog(ctx, freshAccess); err != nil {
		t.Fatal(err)
	}

	res, err := st.ApplyRetention(ctx, RetentionPolicy{EventDays: 30, AccessDays: 30, QuotaBytes: DefaultEventQuotaBytes})
	if err != nil {
		t.Fatal(err)
	}
	if res.EventsDeleted < 1 {
		t.Fatalf("expected old event delete, got %+v", res)
	}
	if res.AccessDeleted != 1 {
		t.Fatalf("access deleted = %d", res.AccessDeleted)
	}

	logs, err := st.ListMachineLogs(ctx, m.ID, "", 20)
	if err != nil {
		t.Fatal(err)
	}
	var md []string
	for _, l := range logs {
		md = append(md, l.Markdown)
	}
	joined := strings.Join(md, "\n")
	if strings.Contains(joined, "old log") {
		t.Fatalf("old log should be gone: %s", joined)
	}
	if !strings.Contains(joined, "fresh log") {
		t.Fatalf("fresh log missing: %s", joined)
	}
	p, err := st.GetPinnedLog(ctx, mustServiceID(t, st, m.ID, "app"))
	if err != nil || p.Markdown != "pinned" {
		t.Fatalf("pin should remain: %v %+v", err, p)
	}
}

func TestEnforceEventQuotaDropsOldestLogs(t *testing.T) {
	ctx := t.Context()
	st := newTestStore(t)
	m := &Machine{MachineKey: "quotabox", Name: "Q", Kind: "vm", Enabled: true, AutoCreateServices: true}
	if err := st.CreateMachine(ctx, m); err != nil {
		t.Fatal(err)
	}
	auth := IngestAuth{MachineID: m.ID, AutoCreateServices: true}
	received := shared.FormatTime(shared.NowUTC())
	if r, err := st.IngestEvent(ctx, mkEnv(t, event.TypeServiceState, "app", "", event.ServiceState{
		Name: "App", Type: "daemon", State: "running", Severity: "normal",
	}), auth, received); err != nil || r.Status != "accepted" {
		t.Fatalf("state: %v %+v", err, r)
	}
	body := strings.Repeat("x", 2000)
	for i := 0; i < 8; i++ {
		env := mkEnv(t, event.TypeLogAppend, "app", "", event.LogPayload{Markdown: body, Severity: "info"})
		env.OccurredAt = shared.FormatTime(shared.NowUTC().Add(time.Duration(i) * time.Minute))
		if r, err := st.IngestEvent(ctx, env, auth, received); err != nil || r.Status != "accepted" {
			t.Fatalf("log %d: %v %+v", i, err, r)
		}
	}
	res, err := st.ApplyRetention(ctx, RetentionPolicy{QuotaBytes: 6000})
	if err != nil {
		t.Fatal(err)
	}
	if res.QuotaDeleted < 1 {
		t.Fatalf("expected quota deletes: %+v", res)
	}
	if res.EventsBytes > 6000 {
		t.Fatalf("still over quota: %+v", res)
	}
	logs, err := st.ListMachineLogs(ctx, m.ID, "", 20)
	if err != nil {
		t.Fatal(err)
	}
	if len(logs) == 0 {
		t.Fatal("quota prune should keep newest logs")
	}
}

func mustServiceID(t *testing.T, st *Store, machineID, key string) string {
	t.Helper()
	svc, err := st.GetServiceByKey(t.Context(), machineID, key)
	if err != nil {
		t.Fatal(err)
	}
	return svc.ID
}
