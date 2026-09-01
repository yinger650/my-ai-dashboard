package aiprovider

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type codexProvider struct {
	exec      ExecFunc
	model     string
	workspace string
}

func (p *codexProvider) Name() string { return "codex" }

func (p *codexProvider) Run(ctx context.Context, req Request) (Result, error) {
	timeout := req.Timeout
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	ws, err := ensureWorkspace(p.workspace)
	if err != nil {
		return Result{}, fmt.Errorf("%w: workspace: %v", ErrUnavailable, err)
	}
	argv := []string{"codex", "exec", "--sandbox", "read-only", "--skip-git-repo-check"}
	if p.model != "" {
		argv = append(argv, "-m", p.model)
	}
	prompt := BuildPrompt(Request{
		Task:       req.Task,
		UserPrompt: req.UserPrompt,
		Untrusted:  Redact(req.Untrusted),
		WantJSON:   req.WantJSON,
		MaxRunes:   req.MaxRunes,
	})
	start := time.Now()
	out, err := runWithTimeout(ctx, timeout, p.exec, argv, prompt, providerEnv(""), ws)
	text := strings.TrimSpace(string(out))
	if strings.Contains(strings.ToLower(text), "authentication required") || strings.Contains(text, "Not logged in") {
		return Result{}, fmt.Errorf("%w: authentication required", ErrUnavailable)
	}
	if err != nil && text == "" {
		return Result{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	if text == "" {
		return Result{}, fmt.Errorf("%w: empty output", ErrUnavailable)
	}
	// Codex prints a session header; keep the last non-empty block after the prompt echo when possible.
	text = lastCodexAnswer(text)
	return Result{
		Text:       clipRunes(text, req.MaxRunes),
		DurationMS: int(time.Since(start).Milliseconds()),
	}, nil
}

func lastCodexAnswer(s string) string {
	const sep = "--------"
	parts := strings.Split(s, sep)
	if len(parts) >= 2 {
		return strings.TrimSpace(parts[len(parts)-1])
	}
	return s
}
