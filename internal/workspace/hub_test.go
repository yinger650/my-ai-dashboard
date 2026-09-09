package workspace

import (
	"path/filepath"
	"testing"
)

func TestEnsureUserAllocatesSlugAndReuses(t *testing.T) {
	dir := t.TempDir()
	ctrl, err := OpenControl(filepath.Join(dir, "control.db"))
	if err != nil {
		t.Fatal(err)
	}
	hub := NewHub(dir, ctrl, nil)
	t.Cleanup(func() { _ = hub.Close() })

	u1, ws1, err := hub.EnsureUser(t.Context(), "ou_1", "on_1", "tk", "Alice", "")
	if err != nil {
		t.Fatal(err)
	}
	if u1.DisplayName != "Alice" || !ValidSlug(ws1.Slug) {
		t.Fatalf("user=%+v ws=%+v", u1, ws1)
	}
	u2, ws2, err := hub.EnsureUser(t.Context(), "ou_1", "on_1", "tk", "Alice 2", "")
	if err != nil {
		t.Fatal(err)
	}
	if u2.ID != u1.ID || ws2.Slug != ws1.Slug {
		t.Fatalf("expected reuse, got %+v %+v", u2, ws2)
	}
	t1, err := hub.Open(t.Context(), ws1.Slug)
	if err != nil || t1.Store == nil {
		t.Fatalf("open: %v %+v", err, t1)
	}
	if _, err := hub.Open(t.Context(), "zzzzzzzz"); err == nil {
		t.Fatal("unknown slug should fail")
	}
}
