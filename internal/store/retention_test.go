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

func TestCloseStaleRunsNoLogsForOneDay(t *testing.T) {
	ctx := t.Context()
	st := newTestStore(t)
	m := &Machine{MachineKey: "stalebox", Name: "Stale", Kind: "virtual", Enabled: true, AutoCreateServices: true}
	if err := st.CreateMachine(ctx, m); err != nil {
		t.Fatal(err)
	}
	auth := IngestAuth{MachineID: m.ID, AutoCreateServices: true}
	now := shared.NowUTC()
	received := shared.FormatTime(now)

	if r, err := st.IngestEvent(ctx, mkEnv(t, event.TypeServiceState, "cursor", "", event.ServiceState{
		Name: "Cursor", Type: "agent", State: "running", Severity: "normal",
	}), auth, received); err != nil || r.Status != "accepted" {
		t.Fatalf("state: %v %+v", err, r)
	}

	seedRun := func(runKey, status string, createdAgo, lastLogAgo time.Duration, withLog bool) {
		t.Helper()
		env := mkEnv(t, event.TypeRunTransition, "cursor", runKey, event.RunTransition{
			ServiceName: "Cursor", ServiceType: "agent", Status: status, Summary: runKey,
			StartedAt: shared.FormatTime(now.Add(-createdAgo)),
		})
		env.OccurredAt = shared.FormatTime(now.Add(-createdAgo))
		if r, err := st.IngestEvent(ctx, env, auth, received); err != nil || r.Status != "accepted" {
			t.Fatalf("run %s: %v %+v", runKey, err, r)
		}
		if !withLog {
			return
		}
		logEnv := mkEnv(t, event.TypeLogAppend, "cursor", runKey, event.LogPayload{
			Markdown: "progress " + runKey, Severity: "info", Source: "cursor",
		})
		logEnv.OccurredAt = shared.FormatTime(now.Add(-lastLogAgo))
		if r, err := st.IngestEvent(ctx, logEnv, auth, received); err != nil || r.Status != "accepted" {
			t.Fatalf("log %s: %v %+v", runKey, err, r)
		}
	}

	seedRun("old-log", "running", 48*time.Hour, 36*time.Hour, true)
	seedRun("fresh-log", "running", 48*time.Hour, 2*time.Hour, true)
	seedRun("no-log-old", "running", 48*time.Hour, 0, false)
	seedRun("no-log-new", "running", 2*time.Hour, 0, false)
	seedRun("queued-old", "queued", 48*time.Hour, 0, false)
	seedRun("done-old", "succeeded", 48*time.Hour, 36*time.Hour, true)

	svcID := mustServiceID(t, st, m.ID, "cursor")
	for _, key := range []string{"old-log", "no-log-old", "queued-old", "done-old"} {
		iso := shared.FormatTime(now.Add(-48 * time.Hour))
		if _, err := st.db.ExecContext(ctx, `UPDATE runs SET created_at = ? WHERE service_id = ? AND run_key = ?`, iso, svcID, key); err != nil {
			t.Fatal(err)
		}
	}

	hb := mkEnv(t, event.TypeServiceState, "cursor", "", event.ServiceState{
		Name: "Cursor", Type: "agent", State: "running", Summary: "", Severity: "normal",
	})
	if r, err := st.IngestEvent(ctx, hb, auth, received); err != nil || r.Status != "accepted" {
		t.Fatalf("heartbeat: %v %+v", err, r)
	}

	before, err := st.GetMachineByID(ctx, m.ID)
	if err != nil {
		t.Fatal(err)
	}

	res, err := st.ApplyRetention(ctx, RetentionPolicy{EventDays: 30, AccessDays: 30, QuotaBytes: DefaultEventQuotaBytes})
	if err != nil {
		t.Fatal(err)
	}
	if res.RunsClosed != 3 {
		t.Fatalf("runs_closed=%d want 3 (%+v)", res.RunsClosed, res)
	}

	svc, err := st.GetServiceByKey(ctx, m.ID, "cursor")
	if err != nil {
		t.Fatal(err)
	}
	runs, err := st.ListRuns(ctx, svc.ID, 20)
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, r := range runs {
		got[r.RunKey] = r.Status
		if r.RunKey == "old-log" {
			if r.FinishedAt == nil || *r.FinishedAt == "" {
				t.Fatalf("old-log missing finished_at: %+v", r)
			}
			if r.DurationMs == nil || *r.DurationMs <= 0 {
				t.Fatalf("old-log missing duration: %+v", r)
			}
		}
	}
	want := map[string]string{
		"old-log":    "timed_out",
		"fresh-log":  "running",
		"no-log-old": "timed_out",
		"no-log-new": "running",
		"queued-old": "cancelled",
		"done-old":   "succeeded",
	}
	for k, status := range want {
		if got[k] != status {
			t.Errorf("%s status=%q want %q (all=%v)", k, got[k], status, got)
		}
	}

	active, err := st.ListActiveRunsByMachine(ctx, m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(active) != 2 {
		t.Fatalf("active runs=%d want 2: %+v", len(active), active)
	}
	if !strings.Contains(svc.StateSummary, "2 进行中") {
		t.Fatalf("summary after close: %q", svc.StateSummary)
	}

	after, err := st.GetMachineByID(ctx, m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if before.LastSeenAt == nil || after.LastSeenAt == nil || *before.LastSeenAt != *after.LastSeenAt {
		t.Fatalf("closing stale runs must not bump machine last_seen: before=%v after=%v", before.LastSeenAt, after.LastSeenAt)
	}

	logs, err := st.ListServiceLogs(ctx, svc.ID, "", 50)
	if err != nil {
		t.Fatal(err)
	}
	var sawClose bool
	for _, l := range logs {
		if l.Markdown == staleRunCloseLog && l.RunKey == "old-log" {
			sawClose = true
			break
		}
	}
	if !sawClose {
		t.Fatalf("expected close log on old-log, logs=%+v", logs)
	}

	res2, err := st.CloseStaleRuns(ctx, DefaultStaleRunIdle)
	if err != nil {
		t.Fatal(err)
	}
	if res2 != 0 {
		t.Fatalf("second close = %d", res2)
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
