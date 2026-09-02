package wrap

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"agentboard/internal/client/control"
)

func TestParseArgs(t *testing.T) {
	opt, argv, err := ParseArgs([]string{"--summary", "训 llama", "--ttl", "6h", "--log", "/tmp/x.log", "--", "scrun", "python", "train.py"})
	if err != nil {
		t.Fatal(err)
	}
	if opt.Summary != "训 llama" || opt.TTL != 6*time.Hour || opt.LogPath != "/tmp/x.log" {
		t.Fatalf("%+v", opt)
	}
	if strings.Join(argv, " ") != "scrun python train.py" {
		t.Fatalf("%v", argv)
	}
}

func TestLogPathIsNotCreated(t *testing.T) {
	dir := t.TempDir()
	sock := filepath.Join(dir, "c.sock")
	logPath := filepath.Join(dir, "job.log")
	var ops []string
	srv, err := control.Listen(sock, func(req control.Request) control.Response {
		ops = append(ops, req.Op)
		return control.Response{OK: true}
	})
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() { _ = srv.Serve(ctx) }()
	time.Sleep(30 * time.Millisecond)

	var out, errb bytes.Buffer
	code, err := Run(Options{
		Sock: sock, Summary: "echo", LogPath: logPath,
		Stdout: &out, Stderr: &errb, Stdin: bytes.NewReader(nil),
	}, []string{"/bin/echo", "hi"})
	if err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("code=%d err=%s", code, errb.String())
	}
	if _, err := os.Stat(logPath); !os.IsNotExist(err) {
		t.Fatalf("--log must be read-only; wrap created %s", logPath)
	}
	joined := strings.Join(ops, ",")
	if !strings.Contains(joined, "wrap_start") || !strings.Contains(joined, "wrap_exit") {
		t.Fatalf("ops=%v", ops)
	}
	if strings.Contains(joined, "wrap_stdout") {
		t.Fatalf(" --log should not pipe stdout to daemon: %v", ops)
	}
	if !strings.Contains(out.String(), "hi") {
		t.Fatalf("stdout=%q", out.String())
	}
}

func TestRunsWithoutDaemon(t *testing.T) {
	var out, errb bytes.Buffer
	code, err := Run(Options{
		Sock: filepath.Join(t.TempDir(), "missing.sock"), Summary: "true",
		Stdout: &out, Stderr: &errb, Stdin: bytes.NewReader(nil),
	}, []string{"/bin/true"})
	if err != nil || code != 0 {
		t.Fatalf("code=%d err=%v %s", code, err, errb.String())
	}
	if !strings.Contains(errb.String(), "daemon not running") {
		t.Fatalf("want warning, got %s", errb.String())
	}
}
