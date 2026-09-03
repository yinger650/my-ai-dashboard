package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCatalogIDsStable(t *testing.T) {
	ids := AllCatalogIDs()
	if len(ids) < 18 {
		t.Fatalf("catalog too small: %v", ids)
	}
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			t.Fatalf("duplicate id %s", id)
		}
		seen[id] = true
	}
	for _, need := range []string{"cpu", "ai.discover", "ai.discover.unit_status", "ai.summarize.agent_logs"} {
		if !seen[need] {
			t.Fatalf("missing %s in %v", need, ids)
		}
	}
}

func TestPresentAndUnseen(t *testing.T) {
	dir := t.TempDir()
	p := filepath.Join(dir, "client.yaml")
	src := `version: 1
server:
  url: "http://127.0.0.1:8080"
  machine_token: "abp_m_x"
machine:
  key: "home-server"
collectors:
  cpu: true
  memory: true
`
	if err := os.WriteFile(p, []byte(src), 0o644); err != nil {
		t.Fatal(err)
	}
	c, err := ReadRawFile(p)
	if err != nil {
		t.Fatal(err)
	}
	present := SeenSet(PresentIDs(c))
	if !present["cpu"] || !present["memory"] {
		t.Fatalf("present=%v", PresentIDs(c))
	}
	if present["ai.discover"] {
		t.Fatal("discover should not be present")
	}
	seen := EffectiveSeen(nil, c)
	unseen := UnseenIDs(seen)
	if !containsStr(unseen, "ai.discover") {
		t.Fatalf("expected ai.discover unseen, got %v", unseen)
	}
	if containsStr(unseen, "cpu") {
		t.Fatalf("cpu should be baseline-seen, got %v", unseen)
	}
	titles := UnseenTitles(seen)
	joined := strings.Join(titles, ",")
	if !strings.Contains(joined, "AI 主机巡检") {
		t.Fatalf("titles=%v", titles)
	}
}

func TestParseSeenRoundTrip(t *testing.T) {
	raw := EncodeSeenIDs([]string{"cpu", "ai.discover", "cpu"})
	got := ParseSeenIDs(raw)
	if len(got) != 2 {
		t.Fatalf("%v", got)
	}
	if ParseSeenIDs(`["memory"]`)[0] != "memory" {
		t.Fatal(ParseSeenIDs(`["memory"]`))
	}
}

func ReadRawFile(path string) (*Config, error) {
	_, c, _, err := LoadDocument(path)
	return c, err
}

func containsStr(ids []string, want string) bool {
	for _, id := range ids {
		if id == want {
			return true
		}
	}
	return false
}
