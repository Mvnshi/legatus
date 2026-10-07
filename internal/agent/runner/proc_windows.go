//go:build windows

package runner

import (
	"os/exec"
	"strconv"
	"syscall"
)

// configure makes cancelling kill the whole process tree: agents start shells and tools of their own,
// and killing only the first process would leave those running.
func configure(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x00000200 /* CREATE_NEW_PROCESS_GROUP */}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		kill := exec.Command("taskkill", "/T", "/F", "/PID", strconv.Itoa(cmd.Process.Pid))
		if err := kill.Run(); err != nil {
			return cmd.Process.Kill()
		}
		return nil
	}
}
