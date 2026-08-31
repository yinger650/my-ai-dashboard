package aiprovider

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

type cursorProvider struct {
	exec      ExecFunc
	model     string
	apiKeyEnv string
	workspace string
}

type cursorResultJSON struct {
	Type       string `json:"type"`
	Subtype    string `json:"subtype"`
	IsError    bool   `json:"is_error"`
	Result     string `json:"result"`
	DurationMS int    `json:"duration_ms"`
	Usage      struct {
		InputTokens     int `json:"inputTokens"`
		OutputTokens    int `json:"outputTokens"`
		CacheReadTokens int `json:"cacheReadTokens"`
	} `json:"usage"`
}

func (p *cursorProvider) Name() string { return "cursor-agent" }

func (p *cursorProvider) Run(ctx context.Context, req Request) (Result, error) {
	timeout := req.Timeout
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	ws, err := ensureWorkspace(p.workspace)
	if err != nil {
		return Result{}, fmt.Errorf("%w: workspace: %v", ErrUnavailable, err)
	}
	argv := []string{"cursor-agent", "-p", "--trust", "--mode", "ask", "--output-format", "json"}
	if p.model != "" {
		argv = append(argv, "--model", p.model)
	}
	prompt := BuildPrompt(Request{
		Task:       req.Task,
		UserPrompt: req.UserPrompt,
		Untrusted:  Redact(req.Untrusted),
		WantJSON:   req.WantJSON,
		MaxRunes:   req.MaxRunes,
	})
	out, err := runWithTimeout(ctx, timeout, p.exec, argv, prompt, providerEnv(p.apiKeyEnv), ws)
	res, parseErr := parseCursorOutput(out)
	if parseErr != nil {
		if err != nil {
			return Result{}, fmt.Errorf("%w: %v", ErrUnavailable, err)
		}
		return Result{}, parseErr
	}
	res.Text = clipRunes(res.Text, req.MaxRunes)
	return res, nil
}

func parseCursorOutput(out []byte) (Result, error) {
	s := strings.TrimSpace(string(out))
	low := strings.ToLower(s)
	if strings.Contains(s, "Workspace Trust Required") || strings.Contains(low, "authentication required") {
		return Result{}, fmt.Errorf("%w: auth or workspace trust required", ErrUnavailable)
	}
	if s == "" {
		return Result{}, fmt.Errorf("%w: empty output", ErrUnavailable)
	}
	lines := strings.Split(s, "\n")
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var raw cursorResultJSON
		if err := json.Unmarshal([]byte(line), &raw); err != nil {
			continue
		}
		if raw.Type != "result" || raw.IsError {
			return Result{}, fmt.Errorf("%w: type=%s error=%v subtype=%s", ErrUnavailable, raw.Type, raw.IsError, raw.Subtype)
		}
		return Result{
			Text:            raw.Result,
			DurationMS:      raw.DurationMS,
			InputTokens:     raw.Usage.InputTokens,
			OutputTokens:    raw.Usage.OutputTokens,
			CacheReadTokens: raw.Usage.CacheReadTokens,
		}, nil
	}
	return Result{}, fmt.Errorf("%w: no result json", ErrUnavailable)
}
