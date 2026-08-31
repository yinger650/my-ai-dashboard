package probe

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"agentboard/internal/event"
)

const defaultMaxBytes = 64 * 1024

// RunScript executes an argv list with a tight env, timeout, and stdout cap.
func RunScript(ctx context.Context, argv []string, timeout time.Duration, maxBytes int) (stdout []byte, truncated bool, err error) {
	if len(argv) == 0 {
		return nil, false, fmt.Errorf("empty argv")
	}
	if err := CheckScript(argv[0]); err != nil {
		return nil, false, err
	}
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	if maxBytes <= 0 {
		maxBytes = defaultMaxBytes
	}
	cctx, cancel := context.WithTimeout(ctx, timeout)
	defer cancel()
	cmd := exec.CommandContext(cctx, argv[0], argv[1:]...)
	cmd.Env = []string{
		"PATH=/usr/sbin:/usr/bin:/sbin:/bin",
		"LANG=C.UTF-8",
		"LC_ALL=C.UTF-8",
		"HOME=" + os.Getenv("HOME"),
	}
	var stdoutBuf, stderrBuf bytes.Buffer
	cmd.Stdout = &stdoutBuf
	cmd.Stderr = &stderrBuf
	runErr := cmd.Run()
	out := stdoutBuf.Bytes()
	if len(out) > maxBytes {
		out = out[:maxBytes]
		truncated = true
	}
	if runErr != nil {
		msg := strings.TrimSpace(stderrBuf.String())
		if msg == "" {
			msg = runErr.Error()
		}
		if truncated {
			msg += " (stdout truncated)"
		}
		return out, truncated, fmt.Errorf("%s", msg)
	}
	return out, truncated, nil
}

func hashMarkdown(md string) string {
	sum := sha256.Sum256([]byte(md))
	return hex.EncodeToString(sum[:])
}

// FailedState maps a script error onto a failed virtual service.
func FailedState(serviceKey, name, msg string, ttl int) []MappedEvent {
	if name == "" {
		name = serviceKey
	}
	return []MappedEvent{{
		Type:       event.TypeServiceState,
		ServiceKey: serviceKey,
		Payload: event.ServiceState{
			Name: name, Type: "virtual", State: "failed",
			Summary: msg, Severity: "error", TTLSeconds: ttlPtr(ttl),
		},
	}, {
		Type:       event.TypeLogAppend,
		ServiceKey: serviceKey,
		Payload:    event.LogPayload{Markdown: msg, Severity: "error", Source: "probe"},
	}}
}
