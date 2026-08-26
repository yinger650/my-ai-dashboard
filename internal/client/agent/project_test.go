package agent

import (
	"testing"

	"agentboard/internal/client/config"
	"agentboard/internal/client/hostsnap"
	"agentboard/internal/event"
)

func meta() Meta {
	return Meta{
		Hostname: "box", HeartbeatSeconds: 30, UptimeSeconds: 100,
		Promote: []config.PromoteRule{{Process: "nginx", ServiceKey: "nginx", Name: "Nginx"}},
	}
}

func eventsOf(evs []Event, typ, key string) []Event {
	var out []Event
	for _, e := range evs {
		if e.Type == typ && (key == "" || e.ServiceKey == key) {
			out = append(out, e)
		}
	}
	return out
}

func TestProjectInspectHasNoOwnLogs(t *testing.T) {
	evs, _ := Project(hostsnap.Snapshot{}, NewState(), meta())
	if n := len(eventsOf(evs, event.TypeLogAppend, InspectKey)); n != 0 {
		t.Fatalf("host-inspect must not append logs, got %d", n)
	}
	if len(eventsOf(evs, event.TypeServiceState, InspectKey)) != 1 {
		t.Fatal("expected inspect service.state")
	}
}

func TestPinDoesNotRepeat(t *testing.T) {
	snap := hostsnap.Snapshot{
		Ports: []hostsnap.Port{{Protocol: "tcp", Address: "0.0.0.0", Port: 80, Process: "nginx"}},
	}
	evs1, st := Project(snap, NewState(), meta())
	pins1 := eventsOf(evs1, event.TypeLogPin, ListenKey)
	if len(pins1) != 1 {
		t.Fatalf("first pin = %d", len(pins1))
	}
	evs2, _ := Project(snap, st, meta())
	if n := len(eventsOf(evs2, event.TypeLogPin, ListenKey)); n != 0 {
		t.Fatalf("unchanged pin should not repeat, got %d", n)
	}
}

func TestNginxHidesUnboundProxy(t *testing.T) {
	snap := hostsnap.Snapshot{
		Ports: []hostsnap.Port{{Protocol: "tcp", Address: "0.0.0.0", Port: 443, Process: "nginx"}},
		Nginx: &hostsnap.Nginx{Available: true, PID: 9, Proxies: []hostsnap.Proxy{
			{ServerName: "live.example", Listen: "443", ListenPort: 443, Location: "/", Upstream: "127.0.0.1:8090"},
			{ServerName: "dead.example", Listen: "8081", ListenPort: 8081, Location: "/", Upstream: "127.0.0.1:9000"},
		}},
	}
	evs, _ := Project(snap, NewState(), meta())
	pins := eventsOf(evs, event.TypeLogPin, NginxKey)
	if len(pins) != 1 {
		t.Fatalf("pins = %d", len(pins))
	}
	md := pins[0].Payload.(event.LogPayload).Markdown
	if !contains(md, "live.example") {
		t.Fatalf("missing live proxy: %s", md)
	}
	if contains(md, "dead.example") {
		t.Fatalf("unbound proxy should be hidden: %s", md)
	}
}

func TestNginxRestartAppendsLog(t *testing.T) {
	prev := NewState()
	prev.NginxPID = 10
	prev.NginxHadProxies = true
	snap := hostsnap.Snapshot{
		Ports: []hostsnap.Port{{Protocol: "tcp", Address: "0.0.0.0", Port: 80, Process: "nginx"}},
		Nginx: &hostsnap.Nginx{Available: true, PID: 22, Proxies: []hostsnap.Proxy{
			{ServerName: "a", Listen: "80", ListenPort: 80, Location: "/", Upstream: "127.0.0.1:1"},
		}},
	}
	evs, _ := Project(snap, prev, meta())
	logs := eventsOf(evs, event.TypeLogAppend, NginxKey)
	if len(logs) != 1 {
		t.Fatalf("restart logs = %d", len(logs))
	}
}

func TestDockerStoppedOnlyCounted(t *testing.T) {
	snap := hostsnap.Snapshot{
		Docker: &hostsnap.Docker{Available: true, ImageCount: 12, Containers: []hostsnap.Container{
			{ID: "aaa", Name: "web", Image: "nginx:latest", State: "running", Ports: "80/tcp"},
			{ID: "bbb", Name: "old", Image: "alpine", State: "exited"},
		}},
	}
	evs, _ := Project(snap, NewState(), meta())
	pins := eventsOf(evs, event.TypeLogPin, DockerKey)
	if len(pins) != 1 {
		t.Fatal("expected docker pin")
	}
	md := pins[0].Payload.(event.LogPayload).Markdown
	if !contains(md, "web") || contains(md, "old") {
		t.Fatalf("running table wrong: %s", md)
	}
	if !contains(md, "停止 1") || !contains(md, "镜像 12") {
		t.Fatalf("counts missing: %s", md)
	}
	if n := len(eventsOf(evs, event.TypeLogAppend, DockerKey)); n != 0 {
		t.Fatalf("first snapshot should not emit docker diffs, got %d", n)
	}

	st := NewState()
	_, st = Project(snap, st, meta())
	snap2 := snap
	snap2.Docker = &hostsnap.Docker{Available: true, ImageCount: 12, Containers: []hostsnap.Container{
		{ID: "aaa", Name: "web", Image: "nginx:latest", State: "exited"},
		{ID: "bbb", Name: "old", Image: "alpine", State: "exited"},
	}}
	evs2, _ := Project(snap2, st, meta())
	if n := len(eventsOf(evs2, event.TypeLogAppend, DockerKey)); n != 1 {
		t.Fatalf("state change logs = %d", n)
	}
}

func TestCronJobsAndRuns(t *testing.T) {
	snap := hostsnap.Snapshot{
		Cron: &hostsnap.Cron{
			Jobs: []hostsnap.CronJob{{Schedule: "0 3 * * *", User: "root", Command: "/usr/bin/backup"}},
			Executions: []hostsnap.CronExec{
				{Key: "e1", Occurred: "2026-08-26T03:00:00Z", User: "root", Command: "/usr/bin/backup"},
			},
		},
	}
	evs, st := Project(snap, NewState(), meta())
	if len(eventsOf(evs, event.TypeLogPin, CronKey)) != 1 {
		t.Fatal("expected cron schedule pin")
	}
	if len(eventsOf(evs, event.TypeRunTransition, CronKey)) != 1 {
		t.Fatal("expected cron run")
	}
	if len(eventsOf(evs, event.TypeLogAppend, CronKey)) != 1 {
		t.Fatal("expected cron result log")
	}
	evs2, _ := Project(snap, st, meta())
	if n := len(eventsOf(evs2, event.TypeRunTransition, CronKey)); n != 0 {
		t.Fatalf("duplicate cron run = %d", n)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || (func() bool {
		for i := 0; i+len(sub) <= len(s); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	})())
}
