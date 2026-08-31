package aiinspect

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"
	"time"

	"agentboard/internal/client/aiprovider"
	"agentboard/internal/client/config"
	"agentboard/internal/client/spool"
	"agentboard/internal/event"
)

type fakeProvider struct {
	name string
	fn   func(aiprovider.Request) (aiprovider.Result, error)
}

func (f fakeProvider) Name() string { return f.name }
func (f fakeProvider) Run(_ context.Context, req aiprovider.Request) (aiprovider.Result, error) {
	return f.fn(req)
}

func TestSummarizeHashSkipAndBudget(t *testing.T) {
	sp, err := spool.Open(t.TempDir() + "/s.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sp.Close() })
	buf := NewBuffer(sp)
	_ = buf.Append(Entry{Markdown: "error boom", Severity: "error", Source: "cursor"})
	_ = buf.Append(Entry{Markdown: "still failing", Severity: "error", Source: "cursor"})
	_ = buf.Append(Entry{Markdown: "retry", Severity: "info", Source: "cursor"})

	calls := 0
	p := fakeProvider{name: "fake", fn: func(req aiprovider.Request) (aiprovider.Result, error) {
		calls++
		if !aiprovider.HasUntrustedFence(aiprovider.BuildPrompt(req)) {
			t.Fatal("caller should use BuildPrompt internally")
		}
		return aiprovider.Result{Text: "## 摘要\n失败了", InputTokens: 10, OutputTokens: 2}, nil
	}}
	ai := config.AIConfig{MaxCallsPerDay: 48, MaxInputBytes: 4096, MaxOutputRunes: 300}
	ai.FallbackHeuristic = boolPtr(true)
	src := config.AISummarize{Source: "agent_logs", ServiceKey: "ai-agent-digest", Name: "Agent 日志总结", MinNewLogs: 3}

	evs := SummarizeOne(context.Background(), sp, buf, p, ai, src)
	if calls != 1 {
		t.Fatalf("calls %d", calls)
	}
	if !hasType(evs, event.TypeLogPin) {
		t.Fatalf("expected pin %+v", evs)
	}
	evs2 := SummarizeOne(context.Background(), sp, buf, p, ai, src)
	if calls != 1 {
		t.Fatalf("hash skip failed, calls=%d evs=%d", calls, len(evs2))
	}

	ai.MaxCallsPerDay = 1
	b := LoadBudget(sp)
	b.Calls = 1
	b.Date = todayUTC()
	SaveBudget(sp, b)
	src.ServiceKey = "other-digest"
	evs3 := SummarizeOne(context.Background(), sp, buf, p, ai, src)
	if !hasNotice(evs3, "ai_budget_exhausted") {
		t.Fatalf("expected budget notice %+v", evs3)
	}
}

func TestSummarizeFallbackUnavailable(t *testing.T) {
	sp, err := spool.Open(t.TempDir() + "/s.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sp.Close() })
	buf := NewBuffer(sp)
	_ = buf.Append(Entry{Markdown: "fatal panic xyz", Severity: "error"})
	p := fakeProvider{fn: func(aiprovider.Request) (aiprovider.Result, error) {
		return aiprovider.Result{}, aiprovider.ErrUnavailable
	}}
	ai := config.AIConfig{MaxCallsPerDay: 10}
	ai.FallbackHeuristic = boolPtr(true)
	src := config.AISummarize{Source: "agent_logs", ServiceKey: "digest", Name: "d", MinNewLogs: 1}
	evs := SummarizeOne(context.Background(), sp, buf, p, ai, src)
	if !hasNotice(evs, "ai_provider_unavailable") {
		t.Fatalf("expected unavailable notice %+v", evs)
	}
	if !hasType(evs, event.TypeLogPin) {
		t.Fatal("heuristic pin missing")
	}
	evs2 := SummarizeOne(context.Background(), sp, buf, p, ai, src)
	if hasNotice(evs2, "ai_provider_unavailable") {
		t.Fatal("notice should be once per day")
	}
}

func TestDiscoverTwoRoundsAndRejectsBadUnit(t *testing.T) {
	sp, err := spool.Open(t.TempDir() + "/s.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sp.Close() })
	tasks := 0
	p := fakeProvider{fn: func(req aiprovider.Request) (aiprovider.Result, error) {
		tasks++
		if req.Task == "triage" {
			return aiprovider.Result{Text: `{"investigate":[{"id":"unit_status","unit":"sshd.service"},{"id":"unit_status","unit":"bad;rm"}]}`, InputTokens: 1}, nil
		}
		if !strings.Contains(req.Untrusted, "rejected") && !strings.Contains(req.Untrusted, "sshd") && !strings.Contains(req.Untrusted, "第一轮") {
			// second round should include first round + investigation
		}
		return aiprovider.Result{Text: "## 巡检\nsshd 正常", InputTokens: 2}, nil
	}}
	fb := true
	ai := config.AIConfig{
		MaxCallsPerDay:    10,
		FallbackHeuristic: &fb,
		Discover: config.AIDiscover{
			Enabled:           true,
			ServiceKey:        "ai-inspect",
			Name:              "AI 主机巡检",
			MaxInvestigations: 8,
			AllowCommands: []config.AllowCmd{
				{ID: "unit_status", Argv: []string{"/bin/echo", "status", "{unit}"}},
			},
		},
	}
	ai.Timeout.Duration = 5 * time.Second
	evs := Discover(context.Background(), sp, p, ai, func(name string, args ...string) ([]byte, error) {
		return []byte(name + " ok\n"), nil
	})
	if tasks != 2 {
		t.Fatalf("rounds=%d", tasks)
	}
	if !hasType(evs, event.TypeLogPin) {
		t.Fatalf("no report %+v", evs)
	}
}

func TestDiscoverUnavailable(t *testing.T) {
	sp, err := spool.Open(t.TempDir() + "/s.db")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sp.Close() })
	p := fakeProvider{fn: func(aiprovider.Request) (aiprovider.Result, error) {
		return aiprovider.Result{}, errors.New("nope")
	}}
	ai := config.AIConfig{MaxCallsPerDay: 5, Discover: config.AIDiscover{Enabled: true, ServiceKey: "ai-inspect"}}
	evs := Discover(context.Background(), sp, p, ai, func(string, ...string) ([]byte, error) { return []byte("x"), nil })
	if !hasNotice(evs, "ai_provider_unavailable") {
		t.Fatalf("%+v", evs)
	}
}

func hasType(evs []OutEvent, typ string) bool {
	for _, e := range evs {
		if e.Type == typ {
			return true
		}
	}
	return false
}

func hasNotice(evs []OutEvent, code string) bool {
	for _, e := range evs {
		if e.Type != event.TypeCollectorNotice {
			continue
		}
		b, _ := json.Marshal(e.Payload)
		var n event.CollectorNotice
		_ = json.Unmarshal(b, &n)
		if n.Code == code {
			return true
		}
	}
	return false
}

func boolPtr(v bool) *bool { return &v }
