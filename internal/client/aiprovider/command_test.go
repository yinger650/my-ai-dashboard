package aiprovider

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestCommandProviderWithFakeAgentScript(t *testing.T) {
	script := filepath.Join("..", "aiinspect", "testdata", "fake-agent.sh")
	if err := os.Chmod(script, 0o755); err != nil {
		t.Fatal(err)
	}
	p, err := New(Options{Provider: "command", Command: []string{mustAbs(t, script)}})
	if err != nil {
		t.Fatal(err)
	}
	res, err := p.Run(context.Background(), Request{Task: "summarize", Untrusted: "error boom", Timeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res.Text, "stub 摘要") {
		t.Fatalf("got %q", res.Text)
	}
	res2, err := p.Run(context.Background(), Request{Task: "triage", WantJSON: true, Untrusted: "sshd running", Timeout: 5 * time.Second})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(res2.Text, `"investigate"`) {
		t.Fatalf("triage %q", res2.Text)
	}
}

func mustAbs(t *testing.T, p string) string {
	t.Helper()
	a, err := filepath.Abs(p)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := exec.LookPath(a); err != nil && true {
		if st, e2 := os.Stat(a); e2 != nil {
			t.Fatal(e2)
		} else if st.Mode()&0o111 == 0 {
			t.Fatalf("not executable: %s", a)
		}
	}
	return a
}
