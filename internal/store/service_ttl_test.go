package store

import (
	"context"
	"testing"
	"time"

	"agentboard/internal/event"
	"agentboard/internal/shared"
)

func TestApplyTTLMarksStale(t *testing.T) {
	ttl := 60
	past := shared.FormatTime(time.Now().UTC().Add(-2 * time.Minute))
	svc := &Service{
		CurrentState: "running",
		StateSummary: "ok",
		Severity:     "normal",
		TTLSeconds:   &ttl,
		LastSeenAt:   &past,
	}
	svc.ApplyTTL(time.Now().UTC())
	if svc.CurrentState != "stale" || svc.Severity != "warning" {
		t.Fatalf("got state=%s sev=%s", svc.CurrentState, svc.Severity)
	}
	if svc.StateSummary != "ok · TTL 过期" {
		t.Fatalf("summary=%q", svc.StateSummary)
	}

	fresh := shared.FormatTime(time.Now().UTC())
	svc2 := &Service{CurrentState: "running", Severity: "normal", TTLSeconds: &ttl, LastSeenAt: &fresh}
	svc2.ApplyTTL(time.Now().UTC())
	if svc2.CurrentState != "running" || svc2.Severity != "normal" {
		t.Fatalf("fresh service should not expire: %+v", svc2)
	}

	failed := &Service{CurrentState: "failed", Severity: "error", TTLSeconds: &ttl, LastSeenAt: &past}
	failed.ApplyTTL(time.Now().UTC())
	if failed.CurrentState != "failed" || failed.Severity != "error" {
		t.Fatalf("failed service should keep terminal state: %+v", failed)
	}
}

func TestServiceTTLProjectedOnRead(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	m := &Machine{MachineKey: "ttlbox", Name: "TTL", Kind: "virtual", Enabled: true, AutoCreateServices: true}
	if err := st.CreateMachine(ctx, m); err != nil {
		t.Fatal(err)
	}
	ttl := 1
	past := shared.FormatTime(time.Now().UTC().Add(-10 * time.Second))
	svc := &Service{
		MachineID: m.ID, ServiceKey: "openclaw", Name: "OpenClaw", Type: "agent",
		CurrentState: "running", Severity: "normal", Enabled: true, TTLSeconds: &ttl, LastSeenAt: &past,
	}
	if err := st.CreateService(ctx, svc); err != nil {
		t.Fatal(err)
	}
	got, err := st.GetServiceByKey(ctx, m.ID, "openclaw")
	if err != nil {
		t.Fatal(err)
	}
	if got.CurrentState != "stale" || got.Severity != "warning" {
		t.Fatalf("read overlay: %+v", got)
	}
	counts, err := st.ServiceSeverityCounts(ctx, m.ID)
	if err != nil {
		t.Fatal(err)
	}
	if counts["warning"] != 1 {
		t.Fatalf("counts=%v", counts)
	}

	now := shared.FormatTime(shared.NowUTC())
	if _, err := st.IngestEvent(ctx, mkEnv(t, event.TypeServiceState, "openclaw", "", event.ServiceState{
		Name: "OpenClaw", Type: "agent", State: "running", Severity: "normal", TTLSeconds: &ttl,
	}), IngestAuth{MachineID: m.ID, AutoCreateServices: true}, now); err != nil {
		t.Fatal(err)
	}
	got, _ = st.GetServiceByKey(ctx, m.ID, "openclaw")
	if got.CurrentState != "running" {
		t.Fatalf("after heartbeat want running, got %s", got.CurrentState)
	}
}
