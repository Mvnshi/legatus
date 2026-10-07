//go:build windows

package engine

import (
	"context"
	"os/exec"
	"syscall"
)

// shellCommand runs a command line through cmd.exe, passing it exactly as written (Go would otherwise
// re-quote it).
func shellCommand(ctx context.Context, command string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "cmd.exe")
	cmd.SysProcAttr = &syscall.SysProcAttr{CmdLine: `cmd.exe /d /s /c "` + command + `"`}
	return cmd
}
