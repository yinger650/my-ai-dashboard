package aiprovider

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type commandProvider struct {
	exec      ExecFunc
	argv      []string
	workspace string
}

func (p *commandProvider) Name() string { return "command" }

func (p *commandProvider) Run(ctx context.Context, req Request) (Result, error) {
	timeout := req.Timeout
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	ws, err := ensureWorkspace(p.workspace)
	if err != nil {
		return Result{}, err
	}
	prompt := BuildPrompt(Request{
		Task:       req.Task,
		UserPrompt: req.UserPrompt,
		Untrusted:  Redact(req.Untrusted),
		WantJSON:   req.WantJSON,
		MaxRunes:   req.MaxRunes,
	})
	start := time.Now()
	out, err := runWithTimeout(ctx, timeout, p.exec, p.argv, prompt, providerEnv(""), ws)
	text := strings.TrimSpace(string(out))
	if err != nil && text == "" {
		return Result{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
	}
	if text == "" {
		return Result{}, fmt.Errorf("%w: empty output", ErrUnavailable)
	}
	return Result{
		Text:       clipRunes(text, req.MaxRunes),
		DurationMS: int(time.Since(start).Milliseconds()),
	}, nil
}
