package spool

import (
	"path/filepath"
	"testing"
)

func TestClientStateRoundTrip(t *testing.T) {
	sp, err := Open(filepath.Join(t.TempDir(), "spool.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = sp.Close() })

	if _, ok, err := sp.GetState("missing"); err != nil || ok {
		t.Fatalf("missing: ok=%v err=%v", ok, err)
	}
	if err := sp.SetState("ai.budget", `{"calls":2}`); err != nil {
		t.Fatal(err)
	}
	v, ok, err := sp.GetState("ai.budget")
	if err != nil || !ok || v != `{"calls":2}` {
		t.Fatalf("got %q ok=%v err=%v", v, ok, err)
	}
	if err := sp.SetState("ai.budget", `{"calls":3}`); err != nil {
		t.Fatal(err)
	}
	v, ok, err = sp.GetState("ai.budget")
	if err != nil || !ok || v != `{"calls":3}` {
		t.Fatalf("update: %q ok=%v err=%v", v, ok, err)
	}
	if err := sp.DeleteState("ai.budget"); err != nil {
		t.Fatal(err)
	}
	if _, ok, err := sp.GetState("ai.budget"); err != nil || ok {
		t.Fatalf("deleted still present ok=%v err=%v", ok, err)
	}
}
