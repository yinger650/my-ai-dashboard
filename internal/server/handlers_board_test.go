package server

import (
	"strings"
	"testing"
	"unicode/utf8"

	"agentboard/internal/store"
)

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
