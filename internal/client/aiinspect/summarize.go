package aiinspect

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"strings"

	"agentboard/internal/client/aiprovider"
	"agentboard/internal/client/collector"
	"agentboard/internal/client/config"
	"agentboard/internal/client/spool"
	"agentboard/internal/event"
	"agentboard/internal/summarize"
)

// OutEvent is a projected Board event.
type OutEvent struct {
	Type       string
	ServiceKey string
	Payload    any
}

// SummarizeOne runs one configured summarize source against the log buffer.
func SummarizeOne(ctx context.Context, sp *spool.Spool, buf *Buffer, p aiprovider.Provider, ai config.AIConfig, src config.AISummarize) []OutEvent {
	bodies, joined := collectSource(buf, src)
	return SummarizeText(ctx, sp, p, ai, src, bodies, joined)
}

// TranscriptBodies joins scanned Cursor transcripts.
func TranscriptBodies(files []collector.Transcript) (bodies []string, joined string) {
	for _, f := range files {
		if strings.TrimSpace(f.Text) == "" {
			continue
		}
		bodies = append(bodies, f.Text)
	}
	return bodies, strings.Join(bodies, "\n")
}

// SummarizeText pins a summary of bodies when content hash changed.
func SummarizeText(ctx context.Context, sp *spool.Spool, p aiprovider.Provider, ai config.AIConfig, src config.AISummarize, bodies []string, hashInput string) []OutEvent {
	if src.MinNewLogs > 0 && len(bodies) < src.MinNewLogs {
		return nil
	}
	if strings.TrimSpace(hashInput) == "" {
		return nil
	}
	sum := sha256.Sum256([]byte(hashInput))
	h := hex.EncodeToString(sum[:])
	hashKey := "ai.hash." + src.ServiceKey
	if prev, ok, _ := sp.GetState(hashKey); ok && prev == h {
		return nil
	}

	b := LoadBudget(sp)
	var evs []OutEvent
	if p != nil && !b.Allow(ai.MaxCallsPerDay) {
		if b.NoticeOnce("ai_budget_exhausted") {
			evs = append(evs, notice("ai_budget_exhausted", "info", "今日 AI 调用次数已达上限，跳过总结。"))
			SaveBudget(sp, b)
		}
		return evs
	}

	name := src.Name
	if name == "" {
		name = src.ServiceKey
	}
	md := ""
	usedAI := false
	if p != nil {
		res, err := p.Run(ctx, aiprovider.Request{
			Task:       "summarize",
			UserPrompt: src.Prompt,
			Untrusted:  clipInput(hashInput, ai.MaxInputBytes),
			Timeout:    ai.Timeout.Duration,
			MaxRunes:   ai.MaxOutputRunes,
		})
		if err == nil && strings.TrimSpace(res.Text) != "" {
			md = res.Text
			usedAI = true
			b.Record(res)
		} else {
			if ai.FallbackHeuristicOn() {
				md = summarize.Logs(name+" 日志总结", bodies)
			}
			if b.NoticeOnce("ai_provider_unavailable") {
				evs = append(evs, notice("ai_provider_unavailable", "info", "AI provider 不可用，已降级为启发式总结。"))
			}
		}
		SaveBudget(sp, b)
	} else if ai.FallbackHeuristicOn() {
		md = summarize.Logs(name+" 日志总结", bodies)
	}
	if strings.TrimSpace(md) == "" {
		return evs
	}
	sev := "info"
	low := strings.ToLower(md)
	if strings.Contains(low, "error") || strings.Contains(md, "失败") || strings.Contains(md, "故障") {
		sev = "warning"
	}
	ttl := 1800
	if src.Interval.Duration > 0 {
		ttl = int(src.Interval.Duration.Seconds()) * 2
		if ttl < 180 {
			ttl = 180
		}
	}
	evs = append(evs,
		OutEvent{Type: event.TypeServiceState, ServiceKey: src.ServiceKey, Payload: event.ServiceState{
			Name: name, Type: "virtual", State: "running", Summary: summaryLine(md), Severity: sev, TTLSeconds: intPtr(ttl),
		}},
		OutEvent{Type: event.TypeLogPin, ServiceKey: src.ServiceKey, Payload: event.LogPayload{
			Markdown: md, Severity: sev, Source: sourceLabel(src.Source, usedAI),
		}},
		OutEvent{Type: event.TypeStatusUpsert, ServiceKey: "board-client", Payload: event.StatusUpsert{Items: b.StatusItems()}},
	)
	if sev == "warning" || sev == "error" {
		evs = append(evs, OutEvent{Type: event.TypeLogAppend, ServiceKey: src.ServiceKey, Payload: event.LogPayload{
			Markdown: md, Severity: sev, Source: sourceLabel(src.Source, usedAI),
		}})
	}
	_ = sp.SetState(hashKey, h)
	return evs
}

func collectSource(buf *Buffer, src config.AISummarize) (bodies []string, joined string) {
	want := src.Source
	if want == "" {
		want = "agent_logs"
	}
	var entries []Entry
	if buf != nil {
		if strings.HasPrefix(want, "probe:") {
			entries = buf.Recent(want, 50)
		} else {
			entries = buf.Recent("agent_logs", 200)
		}
	}
	for _, e := range entries {
		if strings.TrimSpace(e.Markdown) == "" {
			continue
		}
		bodies = append(bodies, e.Markdown)
	}
	return bodies, strings.Join(bodies, "\n")
}

func notice(code, sev, md string) OutEvent {
	return OutEvent{
		Type:    event.TypeCollectorNotice,
		Payload: event.CollectorNotice{Severity: sev, Code: code, Markdown: md},
	}
}

func sourceLabel(src string, usedAI bool) string {
	if src == "" {
		src = "logs"
	}
	if usedAI {
		return "ai-" + src
	}
	return "heuristic-" + src
}

func summaryLine(md string) string {
	for _, line := range strings.Split(md, "\n") {
		line = strings.TrimSpace(strings.TrimLeft(line, "# "))
		if line == "" {
			continue
		}
		r := []rune(line)
		if len(r) > 80 {
			return string(r[:80]) + "…"
		}
		return line
	}
	return "已更新总结"
}

func intPtr(n int) *int { return &n }

func clipInput(s string, maxBytes int) string {
	if maxBytes <= 0 {
		maxBytes = 32 * 1024
	}
	if len(s) <= maxBytes {
		return s
	}
	return "…\n" + s[len(s)-maxBytes:]
}
