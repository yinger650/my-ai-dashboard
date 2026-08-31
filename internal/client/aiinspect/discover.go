package aiinspect

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"agentboard/internal/client/aiprovider"
	"agentboard/internal/client/collector"
	"agentboard/internal/client/config"
	"agentboard/internal/client/probe"
	"agentboard/internal/client/spool"
	"agentboard/internal/event"
)

type investigateDoc struct {
	Investigate []investigateItem `json:"investigate"`
}

type investigateItem struct {
	ID   string `json:"id"`
	Unit string `json:"unit"`
	Path string `json:"path"`
}

// Discover runs the two-round host inspection.
func Discover(ctx context.Context, sp *spool.Spool, p aiprovider.Provider, ai config.AIConfig, run collector.Commander) []OutEvent {
	d := ai.Discover
	if !d.Enabled {
		return nil
	}
	key := d.ServiceKey
	if key == "" {
		key = "ai-inspect"
	}
	name := d.Name
	if name == "" {
		name = "AI 主机巡检"
	}
	ttl := d.TTLSeconds
	if ttl <= 0 {
		ttl = 43200
	}
	maxN := d.MaxInvestigations
	if maxN <= 0 {
		maxN = 8
	}

	b := LoadBudget(sp)
	if p == nil {
		return discoverUnavailable(sp, &b, key, name, ttl)
	}
	if !b.Allow(ai.MaxCallsPerDay) {
		var evs []OutEvent
		if b.NoticeOnce("ai_budget_exhausted") {
			evs = append(evs, notice("ai_budget_exhausted", "info", "今日 AI 调用次数已达上限，跳过巡检。"))
			SaveBudget(sp, b)
		}
		return evs
	}

	round1 := firstRound(run)
	res1, err := p.Run(ctx, aiprovider.Request{
		Task:       "triage",
		UserPrompt: d.Prompt,
		Untrusted:  clipInput(round1, ai.MaxInputBytes),
		WantJSON:   true,
		Timeout:    ai.Timeout.Duration,
		MaxRunes:   ai.MaxOutputRunes,
	})
	if err != nil {
		return discoverUnavailable(sp, &b, key, name, ttl)
	}
	b.Record(res1)
	SaveBudget(sp, b)

	items := parseInvestigate(res1.Text)
	if len(items) > maxN {
		items = items[:maxN]
	}
	round2 := runInvestigations(ctx, d.AllowCommands, items, 20*time.Second)
	if !b.Allow(ai.MaxCallsPerDay) {
		var evs []OutEvent
		if b.NoticeOnce("ai_budget_exhausted") {
			evs = append(evs, notice("ai_budget_exhausted", "info", "今日 AI 调用次数已达上限，跳过巡检第二轮。"))
			SaveBudget(sp, b)
		}
		return evs
	}
	combined := "第一轮清单：\n" + round1 + "\n\n第二轮追查：\n" + round2
	res2, err := p.Run(ctx, aiprovider.Request{
		Task:       "report",
		UserPrompt: d.Prompt,
		Untrusted:  clipInput(combined, ai.MaxInputBytes),
		Timeout:    ai.Timeout.Duration,
		MaxRunes:   ai.MaxOutputRunes,
	})
	if err != nil {
		return discoverUnavailable(sp, &b, key, name, ttl)
	}
	b.Record(res2)
	SaveBudget(sp, b)

	md := strings.TrimSpace(res2.Text)
	if md == "" {
		md = "巡检完成，模型未返回正文。"
	}
	return []OutEvent{
		{Type: event.TypeServiceState, ServiceKey: key, Payload: event.ServiceState{
			Name: name, Type: "virtual", State: "running",
			Summary: summaryLine(md), Severity: "info", TTLSeconds: intPtr(ttl),
		}},
		{Type: event.TypeLogPin, ServiceKey: key, Payload: event.LogPayload{
			Markdown: md, Severity: "info", Source: "ai-inspect",
		}},
		{Type: event.TypeStatusUpsert, ServiceKey: "board-client", Payload: event.StatusUpsert{Items: b.StatusItems()}},
	}
}

func discoverUnavailable(sp *spool.Spool, b *Budget, key, name string, ttl int) []OutEvent {
	var evs []OutEvent
	if b.NoticeOnce("ai_provider_unavailable") {
		evs = append(evs, notice("ai_provider_unavailable", "info", "AI provider 不可用，跳过主机巡检。"))
		SaveBudget(sp, *b)
	}
	evs = append(evs, OutEvent{Type: event.TypeServiceState, ServiceKey: key, Payload: event.ServiceState{
		Name: name, Type: "virtual", State: "stale", Summary: "AI 不可用", Severity: "unknown", TTLSeconds: intPtr(ttl),
	}})
	return evs
}

func firstRound(run collector.Commander) string {
	if run == nil {
		run = collector.DefaultCommander
	}
	type cmd struct {
		title string
		name  string
		args  []string
	}
	cmds := []cmd{
		{"systemctl running", "systemctl", []string{"list-units", "--type=service", "--state=running", "--no-pager", "--plain"}},
		{"top processes", "ps", []string{"-eo", "pid,comm,etime,pcpu,pmem", "--sort=-pcpu"}},
		{"listen ports", "ss", []string{"-tulpnH"}},
	}
	var b strings.Builder
	for _, c := range cmds {
		b.WriteString("### ")
		b.WriteString(c.title)
		b.WriteString("\n")
		out, err := run(c.name, c.args...)
		if err != nil {
			b.WriteString("error: ")
			b.WriteString(err.Error())
			b.WriteString("\n")
			if len(out) > 0 {
				b.Write(out)
				b.WriteString("\n")
			}
			continue
		}
		text := string(out)
		if lines := strings.Split(text, "\n"); len(lines) > 80 {
			text = strings.Join(lines[:80], "\n") + "\n…"
		}
		b.WriteString(aiprovider.Redact(text))
		b.WriteString("\n")
	}
	return b.String()
}

func parseInvestigate(text string) []investigateItem {
	s := strings.TrimSpace(text)
	if i := strings.Index(s, "{"); i >= 0 {
		s = s[i:]
	}
	if j := strings.LastIndex(s, "}"); j >= 0 {
		s = s[:j+1]
	}
	var doc investigateDoc
	if json.Unmarshal([]byte(s), &doc) != nil {
		return nil
	}
	return doc.Investigate
}

func runInvestigations(ctx context.Context, allow []config.AllowCmd, items []investigateItem, timeout time.Duration) string {
	byID := map[string]config.AllowCmd{}
	for _, c := range allow {
		byID[c.ID] = c
	}
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	var b strings.Builder
	for _, it := range items {
		cmd, ok := byID[it.ID]
		if !ok {
			b.WriteString(fmt.Sprintf("skip unknown id %q\n", it.ID))
			continue
		}
		argv, err := probe.Expand(cmd.Argv, map[string]string{"unit": it.Unit, "path": it.Path}, cmd.AllowPaths)
		if err != nil {
			b.WriteString(fmt.Sprintf("rejected %s: %v\n", it.ID, err))
			continue
		}
		cctx, cancel := context.WithTimeout(ctx, timeout)
		out, err := collector.RunCtx(cctx, timeout, argv[0], argv[1:]...)
		cancel()
		b.WriteString("### ")
		b.WriteString(it.ID)
		if it.Unit != "" {
			b.WriteString(" ")
			b.WriteString(it.Unit)
		}
		b.WriteString("\n")
		if err != nil {
			b.WriteString("error: ")
			b.WriteString(err.Error())
			b.WriteString("\n")
		}
		text := aiprovider.Redact(string(out))
		if lines := strings.Split(text, "\n"); len(lines) > 80 {
			text = strings.Join(lines[:80], "\n") + "\n…"
		}
		b.WriteString(text)
		b.WriteString("\n")
	}
	if b.Len() == 0 {
		return "（无追查项）"
	}
	return b.String()
}
