package collector

import (
	"bytes"
	"context"
	"os/exec"
	"time"
)

// Commander runs an external command with a fixed argument list (no shell).
type Commander func(name string, args ...string) ([]byte, error)

// DefaultCommander runs name+args with a 5s timeout.
func DefaultCommander(name string, args ...string) ([]byte, error) {
	return RunCtx(context.Background(), 5*time.Second, name, args...)
}

// RunCtx runs name+args with an optional timeout. Existing collectors keep the 5s default.
func RunCtx(ctx context.Context, timeout time.Duration, name string, args ...string) ([]byte, error) {
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}
	cmd := exec.CommandContext(ctx, name, args...)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	if err != nil {
		if stdout.Len() == 0 && stderr.Len() > 0 {
			return stderr.Bytes(), err
		}
		return stdout.Bytes(), err
	}
	return stdout.Bytes(), nil
}
