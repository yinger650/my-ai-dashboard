package store

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agentboard/internal/event"
	"agentboard/internal/shared"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	st, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open: %v", err)
	}
	if err := st.Migrate(); err != nil {
		t.Fatalf("migrate: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	return st
}

func mkEnv(t *testing.T, etype, serviceKey, runKey string, payload any) *event.Envelope {
	t.Helper()
	pb, _ := json.Marshal(payload)
	return &event.Envelope{
		SchemaVersion: 1,
		EventID:       shared.NewID(),
		EventType:     etype,
		OccurredAt:    shared.FormatTime(shared.NowUTC()),
		ServiceKey:    serviceKey,
		RunKey:        runKey,
		Payload:       pb,
	}
}

func TestIngestProjectionsAndDuplicates(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)

	m := &Machine{MachineKey: "test-box", Name: "Test Box", Kind: "vm", Enabled: true, AutoCreateServices: true}
	if err := st.CreateMachine(ctx, m); err != nil {
		t.Fatalf("create machine: %v", err)
	}
	auth := IngestAuth{MachineID: m.ID, AutoCreateServices: true}
	now := shared.FormatTime(shared.NowUTC())

	// heartbeat
	res, err := st.IngestEvent(ctx, mkEnv(t, event.TypeHeartbeat, "", "", event.Heartbeat{Hostname: "h", OS: "linux", Arch: "amd64", HeartbeatIntervalSeconds: 30}), auth, now)
	if err != nil || res.Status != "accepted" {
		t.Fatalf("heartbeat: %v %+v", err, res)
	}

	// metric
	cpu := 42.0
	memU, memT := int64(4<<30), int64(8<<30)
	res, err = st.IngestEvent(ctx, mkEnv(t, event.TypeMetricSample, "", "", event.MetricSample{CPUPercent: &cpu, MemoryUsedBytes: &memU, MemoryTotalBytes: &memT}), auth, now)
	if err != nil || res.Status != "accepted" {
		t.Fatalf("metric: %v %+v", err, res)
	}
	latest, err := st.LatestMetric(ctx, m.ID)
	if err != nil || latest.CPUPercent == nil || *latest.CPUPercent != 42.0 {
		t.Fatalf("latest metric wrong: %v %+v", err, latest)
	}

	// service.state auto-creates the service
	res, err = st.IngestEvent(ctx, mkEnv(t, event.TypeServiceState, "nginx", "", event.ServiceState{Name: "Nginx", Type: "daemon", State: "running", Severity: "normal"}), auth, now)
	if err != nil || res.Status != "accepted" {
		t.Fatalf("service.state: %v %+v", err, res)
	}
	svc, err := st.GetServiceByKey(ctx, m.ID, "nginx")
	if err != nil {
		t.Fatalf("service not created: %v", err)
	}
	if svc.Severity != "normal" || svc.CurrentState != "running" {
		t.Fatalf("service state not projected: %+v", svc)
	}

	// status.upsert
	res, err = st.IngestEvent(ctx, mkEnv(t, event.TypeStatusUpsert, "nginx", "", event.StatusUpsert{Items: []event.StatusItem{{Key: "q", Label: "queue", Value: json.RawMessage("4"), ValueType: "number", Severity: "warning"}}}), auth, now)
	if err != nil || res.Status != "accepted" {
		t.Fatalf("status: %v %+v", err, res)
	}
	statuses, _ := st.ListStatuses(ctx, svc.ID)
	if len(statuses) != 1 || statuses[0].Severity != "warning" {
		t.Fatalf("status not projected: %+v", statuses)
	}

	// log.append + pin
	if _, err := st.IngestEvent(ctx, mkEnv(t, event.TypeLogAppend, "nginx", "", event.LogPayload{Markdown: "warn log", Severity: "warning", Source: "src"}), auth, now); err != nil {
		t.Fatalf("log.append: %v", err)
	}
	if _, err := st.IngestEvent(ctx, mkEnv(t, event.TypeLogPin, "nginx", "", event.LogPayload{Markdown: "pinned", Severity: "info"}), auth, now); err != nil {
		t.Fatalf("log.pin: %v", err)
	}
	logs, _ := st.ListServiceLogs(ctx, svc.ID, "", 50)
	if len(logs) != 1 || logs[0].Markdown != "warn log" {
		t.Fatalf("logs not projected: %+v", logs)
	}
	pinned, err := st.GetPinnedLog(ctx, svc.ID)
	if err != nil || pinned.Markdown != "pinned" {
		t.Fatalf("pinned not projected: %v %+v", err, pinned)
	}

	// recent machine logs (warning/error only)
	recent, _ := st.RecentMachineLogs(ctx, m.ID, 3)
	if len(recent) != 1 {
		t.Fatalf("recent machine logs = %d, want 1", len(recent))
	}
	if recent[0].ServiceKey != "nginx" {
		t.Fatalf("recent log missing service key: %+v", recent[0])
	}

	pinnedAll, err := st.ListPinnedLogsByMachine(ctx, m.ID)
	if err != nil || len(pinnedAll) != 1 || pinnedAll[0].Markdown != "pinned" {
		t.Fatalf("pinned by machine: %v %+v", err, pinnedAll)
	}
	machStatuses, err := st.ListStatusesByMachine(ctx, m.ID)
	if err != nil || len(machStatuses) != 1 || machStatuses[0].ServiceName == "" {
		t.Fatalf("statuses by machine: %v %+v", err, machStatuses)
	}

	// duplicate detection
	dupEnv := mkEnv(t, event.TypeLogAppend, "nginx", "", event.LogPayload{Markdown: "dup", Severity: "info"})
	if r, _ := st.IngestEvent(ctx, dupEnv, auth, now); r.Status != "accepted" {
		t.Fatalf("first send should accept, got %+v", r)
	}
	if r, _ := st.IngestEvent(ctx, dupEnv, auth, now); r.Status != "duplicate" {
		t.Fatalf("second send should be duplicate, got %+v", r)
	}

	ports := map[string]any{"ports": []map[string]any{{"protocol": "tcp", "address": "0.0.0.0", "port": 80, "process": "nginx"}}}
	if r, err := st.IngestEvent(ctx, mkEnv(t, event.TypePortSnapshot, "", "", ports), auth, now); err != nil || r.Status != "accepted" {
		t.Fatalf("port snapshot: %v %+v", err, r)
	}
	raw, occurred, err := st.LatestPortSnapshot(ctx, m.ID)
	if err != nil || occurred == "" || !strings.Contains(raw, "nginx") {
		t.Fatalf("latest ports: %v %q %s", err, occurred, raw)
	}
}

func TestRunTransitions(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	m := &Machine{MachineKey: "rbox", Name: "R", Kind: "vm", Enabled: true, AutoCreateServices: true}
	_ = st.CreateMachine(ctx, m)
	auth := IngestAuth{MachineID: m.ID, AutoCreateServices: true}
	now := shared.FormatTime(shared.NowUTC())

	runKey := "run-1"
	// queued -> running -> succeeded
	for _, status := range []string{"queued", "running", "succeeded"} {
		r, err := st.IngestEvent(ctx, mkEnv(t, event.TypeRunTransition, "agent", runKey, event.RunTransition{ServiceName: "Agent", ServiceType: "agent", Status: status}), auth, now)
		if err != nil || r.Status != "accepted" {
			t.Fatalf("transition to %s failed: %v %+v", status, err, r)
		}
	}
	// attempt to change terminal state -> rejected invalid_transition
	r, err := st.IngestEvent(ctx, mkEnv(t, event.TypeRunTransition, "agent", runKey, event.RunTransition{ServiceName: "Agent", ServiceType: "agent", Status: "running"}), auth, now)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if r.Status != "rejected" || r.Code != "invalid_transition" {
		t.Fatalf("expected invalid_transition, got %+v", r)
	}

	svc, _ := st.GetServiceByKey(ctx, m.ID, "agent")
	runs, _ := st.ListRuns(ctx, svc.ID, 10)
	if len(runs) != 1 || runs[0].Status != "succeeded" {
		t.Fatalf("run not in succeeded terminal state: %+v", runs)
	}
}

func TestServiceTokenCannotAutoCreateStatus(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	m := &Machine{MachineKey: "sbox", Name: "S", Kind: "vm", Enabled: true, AutoCreateServices: false}
	_ = st.CreateMachine(ctx, m)
	auth := IngestAuth{MachineID: m.ID, AutoCreateServices: false}
	now := shared.FormatTime(shared.NowUTC())

	// status.upsert to unknown service with auto-create disabled -> not_found
	r, err := st.IngestEvent(ctx, mkEnv(t, event.TypeStatusUpsert, "ghost", "", event.StatusUpsert{Items: []event.StatusItem{{Key: "k", Label: "l", Value: json.RawMessage("1"), ValueType: "number", Severity: "info"}}}), auth, now)
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if r.Status != "rejected" || r.Code != "not_found" {
		t.Fatalf("expected not_found, got %+v", r)
	}
}

func TestListMachineLogsPagination(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	m := &Machine{MachineKey: "logbox", Name: "Log Box", Kind: "vm", Enabled: true, AutoCreateServices: true}
	if err := st.CreateMachine(ctx, m); err != nil {
		t.Fatalf("create: %v", err)
	}
	auth := IngestAuth{MachineID: m.ID, AutoCreateServices: true}
	now := shared.NowUTC()
	received := shared.FormatTime(now)
	if r, err := st.IngestEvent(ctx, mkEnv(t, event.TypeServiceState, "app", "", event.ServiceState{
		Name: "App", Type: "daemon", State: "running", Severity: "normal",
	}), auth, received); err != nil || r.Status != "accepted" {
		t.Fatalf("service.state: %v %+v", err, r)
	}

	base := now.Add(-time.Minute)
	for i := 0; i < 5; i++ {
		env := mkEnv(t, event.TypeLogAppend, "app", "", event.LogPayload{
			Markdown: fmt.Sprintf("line-%d", i),
			Severity: "info",
		})
		env.OccurredAt = shared.FormatTime(base.Add(time.Duration(i) * time.Second))
		if r, err := st.IngestEvent(ctx, env, auth, received); err != nil || r.Status != "accepted" {
			t.Fatalf("append %d: %v %+v", i, err, r)
		}
	}

	page, err := st.ListMachineLogs(ctx, m.ID, "", 2)
	if err != nil || len(page) != 2 {
		t.Fatalf("first page: %v n=%d", err, len(page))
	}
	if page[0].Markdown != "line-4" || page[1].Markdown != "line-3" {
		t.Fatalf("newest-first mismatch: %+v", page)
	}
	older, err := st.ListMachineLogs(ctx, m.ID, page[1].OccurredAt, 2)
	if err != nil || len(older) != 2 {
		t.Fatalf("second page: %v n=%d", err, len(older))
	}
	if older[0].Markdown != "line-2" {
		t.Fatalf("cursor did not continue: %+v", older)
	}
	all, err := st.ListMachineLogs(ctx, m.ID, "", 50)
	if err != nil || len(all) != 5 {
		t.Fatalf("all logs: %v n=%d", err, len(all))
	}
}
