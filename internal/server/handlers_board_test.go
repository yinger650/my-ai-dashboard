package server

import (
	"testing"

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
}
