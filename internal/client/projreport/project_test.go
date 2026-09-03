package projreport

import (
	"encoding/json"
	"testing"

	"agentboard/internal/event"
	"agentboard/internal/shared"
)

func TestKeyAndName(t *testing.T) {
	key, name := KeyAndName("/mnt/afs/wangmin/code/my-ai-dashboard")
	if name != "my-ai-dashboard" {
		t.Fatalf("name=%q", name)
	}
	if key != "proj-my-ai-dashboard" {
		t.Fatalf("key=%q", key)
	}
}

func TestProjectRewritesOntoProjService(t *testing.T) {
	st := event.ServiceState{
		Name: "Cursor Agent", Type: "agent", State: "running", Severity: "normal",
		Metadata: map[string]any{"provider": "cursor", "workspace": "/repo/my-ai-dashboard", "project": "my-ai-dashboard"},
	}
	raw, _ := json.Marshal(st)
	env := event.Envelope{
		SchemaVersion: 1,
		EventID:       shared.NewID(),
		EventType:     event.TypeServiceState,
		ServiceKey:    "cursor",
		Payload:       raw,
	}
	out := Project(env)
	if len(out) != 2 {
		t.Fatalf("len=%d", len(out))
	}
	if out[0].ServiceKey != "proj-my-ai-dashboard" {
		t.Fatalf("key=%q", out[0].ServiceKey)
	}
	got, ok := out[0].Payload.(event.ServiceState)
	if !ok || got.Name != "my-ai-dashboard" {
		t.Fatalf("payload=%#v", out[0].Payload)
	}
	if got.Metadata["host_project"] != true {
		t.Fatalf("metadata=%v", got.Metadata)
	}
	if got.Metadata["path"] != "/repo/my-ai-dashboard" {
		t.Fatalf("path=%v", got.Metadata["path"])
	}
	if out[1].Type != event.TypeStatusUpsert {
		t.Fatalf("extra type %s", out[1].Type)
	}
}

func TestProjectSkipsWithoutWorkspace(t *testing.T) {
	raw, _ := json.Marshal(event.LogPayload{Markdown: "hi", Severity: "info", Source: "cursor"})
	env := event.Envelope{EventType: event.TypeLogAppend, ServiceKey: "cursor", Payload: raw}
	if Project(env) != nil {
		t.Fatal("expected skip")
	}
}

func TestProjectRunAndLog(t *testing.T) {
	rt := event.RunTransition{
		ServiceName: "Cursor Agent", ServiceType: "agent", Status: "running", Summary: "fix ingest",
		Metadata: map[string]any{"workspace": "/x/demo", "project": "demo"},
	}
	raw, _ := json.Marshal(rt)
	out := Project(event.Envelope{EventType: event.TypeRunTransition, ServiceKey: "cursor", RunKey: "run-1", Payload: raw})
	if len(out) != 1 || out[0].ServiceKey != "proj-demo" || out[0].RunKey != "run-1" {
		t.Fatalf("%#v", out)
	}
	got := out[0].Payload.(event.RunTransition)
	if got.ServiceName != "demo" {
		t.Fatalf("name=%q", got.ServiceName)
	}

	lpRaw, _ := json.Marshal(map[string]any{
		"markdown": "start", "severity": "info", "source": "cursor",
		"metadata": map[string]any{"workspace": "/x/demo"},
	})
	logs := Project(event.Envelope{EventType: event.TypeLogAppend, ServiceKey: "cursor", Payload: lpRaw})
	if len(logs) != 1 || logs[0].ServiceKey != "proj-demo" {
		t.Fatalf("log %#v", logs)
	}
}
