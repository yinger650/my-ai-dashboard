package summarize

import "testing"

func TestLogsIncludesErrors(t *testing.T) {
	out := Logs("Agent 运行", []string{
		"started job",
		"ERROR: boom failed to connect",
		"warning: retrying",
		"done",
	})
	if out == "" || !contains(out, "ERROR") || !contains(out, "日志条数：4") {
		t.Fatalf("unexpected summary:\n%s", out)
	}
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(sub) == 0 || (len(s) > 0 && (index(s, sub) >= 0)))
}

func index(s, sub string) int {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return i
		}
	}
	return -1
}
