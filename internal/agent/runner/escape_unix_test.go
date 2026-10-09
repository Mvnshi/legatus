//go:build !windows

package runner

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"testing"
	"time"
)

// startEscapee is the "agent" of TestCancelAlsoStopsWhatTheAgentStartedInASessionOfItsOwn: it starts a
// long-running child in a session of its own, as Claude Code does for the commands of its shell tool, says
// where the child is, and waits.
func startEscapee() int {
	exe, err := os.Executable()
	if err != nil {
		return 99
	}
	child := exec.Command(exe)
	child.Env = append(os.Environ(), "LEGATUS_RUNNER_MODE=sleep")
	child.SysProcAttr = &syscall.SysProcAttr{Setsid: true}
	if err := child.Start(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 99
	}
	if err := os.WriteFile(filepath.Join(os.Getenv("LEGATUS_RUNNER_DIR"), "child.pid"), []byte(strconv.Itoa(child.Process.Pid)), 0o600); err != nil {
		return 99
	}
	fmt.Fprintln(os.Stdout, "ready")
	time.Sleep(time.Minute)
	return 0
}

// alive reports whether the process is running. A zombie (dead, not yet collected by its new parent) is not.
func alive(pid int) bool {
	if err := syscall.Kill(pid, 0); err != nil {
		return false
	}
	out, err := exec.Command("ps", "-o", "stat=", "-p", strconv.Itoa(pid)).Output()
	if err != nil {
		return false
	}
	state := strings.TrimSpace(string(out))
	return state != "" && !strings.HasPrefix(state, "Z")
}

func TestCancelAlsoStopsWhatTheAgentStartedInASessionOfItsOwn(t *testing.T) {
	if _, err := exec.LookPath("ps"); err != nil {
		t.Skip("ps is not installed, so the test cannot tell whether the child is still alive")
	}
	spec := helperSpec(t, "", "", "escape")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	_, err := Stream(ctx, spec, func(line []byte) {
		if string(line) == "ready" {
			cancel()
		}
	})
	if err == nil {
		t.Fatal("the run was not cancelled")
	}
	data, rerr := os.ReadFile(filepath.Join(spec.Dir, "child.pid"))
	if rerr != nil {
		t.Fatalf("the agent did not start its child: %v", rerr)
	}
	pid, _ := strconv.Atoi(string(data))
	t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })
	deadline := time.Now().Add(5 * time.Second)
	for alive(pid) {
		if time.Now().After(deadline) {
			t.Fatalf("process %d, started by the agent in its own session, is still running after the run was cancelled", pid)
		}
		time.Sleep(50 * time.Millisecond)
	}
}
