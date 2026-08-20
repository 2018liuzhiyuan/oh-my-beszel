package agent

import (
	"context"
	"os/exec"
	"time"
)

// runCommandOutput runs a command with a short timeout and returns its stdout.
// Used for optional one-shot probes (ipmitool etc.) where failure is expected
// and should not be fatal.
func runCommandOutput(name string, args ...string) (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, name, args...).Output()
	if err != nil {
		return "", err
	}
	return string(out), nil
}
