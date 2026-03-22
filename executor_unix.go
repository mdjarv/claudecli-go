//go:build !windows

package claudecli

import (
	"context"
	"os/exec"
	"runtime"
	"syscall"
	"time"
)

func setPlatformAttrs(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGTERM)
	}
	cmd.WaitDelay = 5 * time.Second
}

// buildPlatformCmd creates the exec.Cmd with platform-specific handling.
// On Linux, wraps with stdbuf -oL to force line-buffered stdout when available.
func buildPlatformCmd(ctx context.Context, binary string, args []string) *exec.Cmd {
	if runtime.GOOS == "linux" {
		if stdbuf, err := exec.LookPath("stdbuf"); err == nil {
			return exec.CommandContext(ctx, stdbuf, append([]string{"-oL", binary}, args...)...)
		}
	}
	return exec.CommandContext(ctx, binary, args...)
}
