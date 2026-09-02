package wrapd

import (
	"strings"
	"testing"
	"time"

	"agentboard/internal/client/control"
	"agentboard/internal/event"
)

type queued struct {
	Type, Service, RunKey string
	Payload               any
}

func setup() (*Manager, *[]queued, *[]string) {
	var evs []queued
	var audits []string
	m := New()
	m.Enqueue = func(t, sk, rk string, p any) {
		evs = append(evs, queued{t, sk, rk, p})
	}
	m.Audit = func(rk, sk, st, sum string) {
		audits = append(audits, rk+"|"+sk+"|"+st+"|"+sum)
	}
	m.Now = func() time.Time { return time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC) }
	return m, &evs, &audits
}

func TestWrapStartUsesBoardClient(t *testing.T) {
	m, evs, _ := setup()
	resp := m.Handle(control.Request{Op: "wrap_start", RunKey: "rk1", PID: 9, Summary: "训 llama"})
	if !resp.OK {
		t.Fatalf("%+v", resp)
	}
	if len(*evs) != 1 || (*evs)[0].Service != "board-client" || (*evs)[0].Type != event.TypeRunTransition {
		t.Fatalf("%+v", *evs)
	}
	rt := (*evs)[0].Payload.(event.RunTransition)
	if rt.Status != "running" || rt.Summary != "训 llama" {
		t.Fatalf("%+v", rt)
	}
}

func TestTimedOutThenExitSkipsTransition(t *testing.T) {
	m, evs, audits := setup()
	base := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	now := base
	m.Now = func() time.Time { return now }
	m.Handle(control.Request{Op: "wrap_start", RunKey: "rk1", Summary: "job", TTLSeconds: 1})
	now = base.Add(2 * time.Second)
	m.TickTTL()
	n := len(*evs)
	a := len(*audits)
	code := 0
	resp := m.Handle(control.Request{Op: "wrap_exit", RunKey: "rk1", ExitCode: &code})
	if !resp.OK {
		t.Fatal(resp)
	}
	if len(*evs) != n {
		t.Fatalf("late exit must not enqueue again: %+v", *evs)
	}
	if len(*audits) != a {
		t.Fatalf("late exit must not re-audit: %v", *audits)
	}
	foundTO := false
	for _, e := range *evs {
		if rt, ok := e.Payload.(event.RunTransition); ok && rt.Status == "timed_out" {
			foundTO = true
		}
	}
	if !foundTO {
		t.Fatalf("missing timed_out: %+v", *evs)
	}
}

func TestExitFailedAuditsOnce(t *testing.T) {
	m, evs, audits := setup()
	m.Handle(control.Request{Op: "wrap_start", RunKey: "rk1", Summary: "sensecore 训 llama"})
	code := 1
	m.Handle(control.Request{Op: "wrap_exit", RunKey: "rk1", ExitCode: &code})
	m.Handle(control.Request{Op: "wrap_exit", RunKey: "rk1", ExitCode: &code})
	if len(*audits) != 1 {
		t.Fatalf("audits=%v", *audits)
	}
	if !strings.Contains((*audits)[0], "board-client") || !strings.Contains((*audits)[0], "failed") {
		t.Fatalf("%v", *audits)
	}
	var terminals int
	for _, e := range *evs {
		if rt, ok := e.Payload.(event.RunTransition); ok && event.IsTerminal(rt.Status) {
			terminals++
			if e.Service != "board-client" {
				t.Fatalf("service %s", e.Service)
			}
		}
	}
	if terminals != 1 {
		t.Fatalf("terminal transitions=%d evs=%+v", terminals, *evs)
	}
}

func TestEmptyStdoutDoesNotSummarize(t *testing.T) {
	m, _, _ := setup()
	var sums int
	m.Summarize = func(string, string) { sums++ }
	m.Handle(control.Request{Op: "wrap_start", RunKey: "rk1", Summary: "x"})
	m.Handle(control.Request{Op: "wrap_stdout", RunKey: "rk1", Chunk: ""})
	if sums != 0 {
		t.Fatal("empty chunk must not summarize")
	}
}
