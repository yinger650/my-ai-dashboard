package store

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"agentboard/internal/event"
	"agentboard/internal/shared"
)

func TestServiceSnapshotProjectsUnits(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	m := &Machine{MachineKey: "sysbox", Name: "Sys", Kind: "vm", Enabled: true, AutoCreateServices: true}
	if err := st.CreateMachine(ctx, m); err != nil {
		t.Fatal(err)
	}
	auth := IngestAuth{MachineID: m.ID, AutoCreateServices: true}
	now := shared.FormatTime(shared.NowUTC())

	res, err := st.IngestEvent(ctx, mkEnv(t, event.TypeServiceSnapshot, "", "", event.ServiceSnapshot{
		Units: []event.SnapshotUnit{
			{Unit: "nginx.service", Active: "active", Sub: "running", Description: "nginx"},
			{Unit: "sshd.service", Active: "failed", Sub: "failed", Description: "sshd"},
			{Unit: "bad unit", Active: "active"}, // skipped: invalid key
		},
	}), auth, now)
	if err != nil || res.Status != "accepted" {
		t.Fatalf("snapshot: %v %+v", err, res)
	}

	nginx, err := st.GetServiceByKey(ctx, m.ID, "nginx.service")
	if err != nil {
		t.Fatalf("nginx not created: %v", err)
	}
	if nginx.Type != "daemon" || nginx.CurrentState != "running" || nginx.Severity != "normal" {
		t.Fatalf("nginx projection: %+v", nginx)
	}
	sshd, err := st.GetServiceByKey(ctx, m.ID, "sshd.service")
	if err != nil {
		t.Fatalf("sshd not created: %v", err)
	}
	if sshd.CurrentState != "failed" || sshd.Severity != "error" {
		t.Fatalf("sshd projection: %+v", sshd)
	}
	if _, err := st.GetServiceByKey(ctx, m.ID, "bad unit"); err == nil {
		t.Fatal("invalid unit key should not create a service")
	}

	// update existing
	res, err = st.IngestEvent(ctx, mkEnv(t, event.TypeServiceSnapshot, "", "", event.ServiceSnapshot{
		Units: []event.SnapshotUnit{{Unit: "nginx.service", Active: "inactive", Sub: "dead"}},
	}), auth, now)
	if err != nil || res.Status != "accepted" {
		t.Fatalf("update: %v %+v", err, res)
	}
	nginx, _ = st.GetServiceByKey(ctx, m.ID, "nginx.service")
	if nginx.CurrentState != "stopped" {
		t.Fatalf("nginx should be stopped, got %s", nginx.CurrentState)
	}
}

func TestArtifactQuota(t *testing.T) {
	ctx := context.Background()
	st := newTestStore(t)
	m := &Machine{MachineKey: "abox", Name: "A", Kind: "vm", Enabled: true}
	if err := st.CreateMachine(ctx, m); err != nil {
		t.Fatal(err)
	}
	svc := &Service{MachineID: m.ID, ServiceKey: "svc", Name: "Svc", Type: "daemon", Enabled: true}
	if err := st.CreateService(ctx, svc); err != nil {
		t.Fatal(err)
	}
	a := &Artifact{
		UploadEventID: shared.NewID(),
		MachineID:     m.ID,
		ServiceID:     &svc.ID,
		StoredName:    "a.bin",
		OriginalName:  "a.bin",
		MIMEType:      "application/octet-stream",
		SizeBytes:     100,
		SHA256:        "abc",
	}
	if err := st.InsertArtifact(ctx, a); err != nil {
		t.Fatal(err)
	}
	used, err := st.ArtifactBytesUsed(ctx)
	if err != nil || used != 100 {
		t.Fatalf("used=%d err=%v", used, err)
	}
	list, err := st.ListArtifactsByService(ctx, svc.ID, 10)
	if err != nil || len(list) != 1 {
		t.Fatalf("list=%v err=%v", list, err)
	}
	if err := st.SoftDeleteArtifact(ctx, a.ID); err != nil {
		t.Fatal(err)
	}
	used, _ = st.ArtifactBytesUsed(ctx)
	if used != 0 {
		t.Fatalf("deleted artifact still counted: %d", used)
	}
}

func TestLoadSecretKeyFile(t *testing.T) {
	dir := t.TempDir()
	// just ensure artifacts dir exists for related tests
	if err := os.MkdirAll(filepath.Join(dir, "artifacts"), 0o750); err != nil {
		t.Fatal(err)
	}
}
