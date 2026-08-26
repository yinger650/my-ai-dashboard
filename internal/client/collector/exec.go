package collector

import (
	"context"
	"os/exec"
	"time"
)

// Commander runs an external command with a fixed argument list (no shell).
type Commander func(name string, args ...string) ([]byte, error)

// DefaultCommander runs name+args with a 5s timeout.
func DefaultCommander(name string, args ...string) ([]byte, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return exec.CommandContext(ctx, name, args...).Output()
}
