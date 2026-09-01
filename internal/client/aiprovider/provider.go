// Package aiprovider shells out to a local coding-agent CLI for log summaries.
package aiprovider

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"
	"unicode/utf8"
)

// ErrUnavailable means the CLI is missing, unauthenticated, or returned a non-result.
var ErrUnavailable = errors.New("ai provider unavailable")

// Request is one model call. UserPrompt cannot replace the fixed system prefix.
type Request struct {
	Task       string // summarize | triage | report
	UserPrompt string
	Untrusted  string
	WantJSON   bool
	Timeout    time.Duration
	MaxRunes   int
}

// Result is the model text plus optional usage from cursor-agent JSON.
type Result struct {
	Text            string
	DurationMS      int
	InputTokens     int
	OutputTokens    int
	CacheReadTokens int
}

// Provider runs one AI request.
type Provider interface {
	Name() string
	Run(ctx context.Context, req Request) (Result, error)
}

// ExecFunc runs argv with stdin. Tests inject a fake.
type ExecFunc func(ctx context.Context, argv []string, stdin string, env []string, workdir string) ([]byte, error)

// Options construct a named provider.
type Options struct {
	Provider  string
	Command   []string
	Model     string
	APIKeyEnv string
	Workspace string
	Exec      ExecFunc
}

// New returns a provider for the configured name.
func New(opt Options) (Provider, error) {
	execFn := opt.Exec
	if execFn == nil {
		execFn = DefaultExec
	}
	ws := opt.Workspace
	switch strings.ToLower(strings.TrimSpace(opt.Provider)) {
	case "", "cursor-agent", "cursor":
		return &cursorProvider{exec: execFn, model: opt.Model, apiKeyEnv: defaultKeyEnv(opt.APIKeyEnv), workspace: ws}, nil
	case "codex":
		return &codexProvider{exec: execFn, model: opt.Model, workspace: ws}, nil
	case "command":
		if len(opt.Command) == 0 {
			return nil, fmt.Errorf("ai.command is required when provider=command")
		}
		return &commandProvider{exec: execFn, argv: append([]string(nil), opt.Command...), workspace: ws}, nil
	default:
		return nil, fmt.Errorf("unknown ai.provider %q", opt.Provider)
	}
}

func defaultKeyEnv(v string) string {
	if v == "" {
		return "CURSOR_API_KEY"
	}
	return v
}

// DefaultExec runs argv with no shell. Combined stdout+stderr so trust/auth banners are visible.
func DefaultExec(ctx context.Context, argv []string, stdin string, env []string, workdir string) ([]byte, error) {
	if len(argv) == 0 {
		return nil, fmt.Errorf("empty argv")
	}
	cmd := exec.CommandContext(ctx, argv[0], argv[1:]...)
	if stdin != "" {
		cmd.Stdin = strings.NewReader(stdin)
	}
	if workdir != "" {
		cmd.Dir = workdir
	}
	if len(env) > 0 {
		cmd.Env = env
	}
	var buf bytes.Buffer
	cmd.Stdout = &buf
	cmd.Stderr = &buf
	err := cmd.Run()
	return buf.Bytes(), err
}

func runWithTimeout(ctx context.Context, timeout time.Duration, execFn ExecFunc, argv []string, stdin string, env []string, workdir string) ([]byte, error) {
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	return execFn(ctx, argv, stdin, env, workdir)
}

func clipRunes(s string, n int) string {
	if n <= 0 {
		n = 3000
	}
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n]) + "\n…"
}

func providerEnv(apiKeyEnv string) []string {
	env := []string{
		"PATH=" + os.Getenv("PATH"),
		"HOME=" + os.Getenv("HOME"),
		"LANG=C.UTF-8",
		"LC_ALL=C.UTF-8",
	}
	if user := os.Getenv("USER"); user != "" {
		env = append(env, "USER="+user)
	}
	if apiKeyEnv != "" {
		if v := os.Getenv(apiKeyEnv); v != "" {
			env = append(env, apiKeyEnv+"="+v)
		}
	}
	return env
}

func ensureWorkspace(dir string) (string, error) {
	if dir == "" {
		return "", nil
	}
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return "", err
	}
	return dir, nil
}
