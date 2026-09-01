package server

import (
	"strings"
	"testing"
	"unicode/utf8"

	"agentboard/internal/store"
)

func TestCardRecentLogsSkipCron(t *testing.T) {
	got := cardRecentLogs([]store.LogEntry{
		{Markdown: "cron", Source: "cron"},
		{Markdown: "also cron", ServiceKey: "cron"},
		{Markdown: "nginx", Source: "nginx"},
	}, 20)
	if len(got) != 1 || got[0].Markdown != "nginx" {
		t.Fatalf("%+v", got)
	}
}

func TestCardPinsKeepCurrentState(t *testing.T) {
	got := cardPins([]store.PinnedLog{
		{ServiceKey: "cursor-agent", Markdown: "agent"},
		{ServiceKey: "nginx", Markdown: "proxy"},
		{ServiceKey: "host-listen", Markdown: "ports"},
	})
	if len(got) != 2 {
		t.Fatalf("%+v", got)
	}
}

func TestKeepTextBoardStatus(t *testing.T) {
	if keepTextBoardStatus(store.CurrentStatus{StatusKey: "alive", Severity: "error"}) {
		t.Fatal("telemetry should stay hidden")
	}
	if keepTextBoardStatus(store.CurrentStatus{StatusKey: "probe", Severity: "normal"}) {
		t.Fatal("healthy probe should stay hidden")
	}
	if !keepTextBoardStatus(store.CurrentStatus{StatusKey: "probe", Severity: "error"}) {
		t.Fatal("down probe should show")
	}
	parts := textBoardStatusParts([]store.CurrentStatus{
		{StatusKey: "alive", Label: "存活", Severity: "normal", ServiceKey: "cursor", ValueJSON: "true"},
		{StatusKey: "ssl_days", Label: "证书", Severity: "warning", ServiceKey: "site-web", ValueJSON: "8"},
	})
	if len(parts) != 1 || !strings.Contains(parts[0], "证书") {
		t.Fatalf("%q", parts)
	}
}

func TestStatusValueText(t *testing.T) {
	unit := "%"
	got := statusValueText(store.CurrentStatus{ValueJSON: "12.5", Unit: &unit})
	if got != "12.5%" {
		t.Fatalf("got %q", got)
	}
	got = statusValueText(store.CurrentStatus{ValueJSON: `"running"`})
	if got != "running" {
		t.Fatalf("got %q", got)
	}
}

func TestOneLineTruncates(t *testing.T) {
	if got := oneLine("hello\nworld"); got != "hello world" {
		t.Fatalf("newline: %q", got)
	}
	long := ""
	for i := 0; i < 40; i++ {
		long += "abcd "
	}
	got := oneLine(long)
	if len(got) <= 100 {
		t.Fatalf("expected truncation, got len=%d %q", len(got), got)
	}
	if got[len(got)-len("…"):] != "…" {
		t.Fatalf("missing ellipsis: %q", got)
	}
	cn := strings.Repeat("探测失败", 40)
	got = oneLine(cn)
	if !utf8.ValidString(got) {
		t.Fatalf("truncated chinese is not valid utf-8: %q", got)
	}
	if utf8.RuneCountInString(got) != 101 { // 100 runes + ellipsis
		t.Fatalf("rune count = %d", utf8.RuneCountInString(got))
	}
}
