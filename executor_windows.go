//go:build windows

package claudecli

import (
	"context"
	"os/exec"
)

func setPlatformAttrs(cmd *exec.Cmd) {
	// On Windows, use the default behavior: cmd.Process.Kill() on context cancel.
	// No process group management available.
}

// buildPlatformCmd creates the exec.Cmd. No special wrapping needed on Windows.
func buildPlatformCmd(ctx context.Context, binary string, args []string) *exec.Cmd {
	return exec.CommandContext(ctx, binary, args...)
}
