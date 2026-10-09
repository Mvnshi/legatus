//go:build !windows

package runner

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
)

// configure puts the agent in its own process group and makes cancelling stop everything it started.
//
// The group alone is not enough: Claude Code runs the commands of its shell tool in a session of their own, so
// a group kill stops the agent and leaves a long-running command behind, owned by nobody. Cancelling
// therefore lists the agent's descendants first, then kills the group and each of them.
func configure(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		pid := cmd.Process.Pid
		kids := descendants(pid)
		err := syscall.Kill(-pid, syscall.SIGKILL)
		if err != nil {
			err = cmd.Process.Kill()
		}
		for _, k := range kids {
			_ = syscall.Kill(k, syscall.SIGKILL)
		}
		return err
	}
}

// descendants are the processes started, directly or through others, by pid and still alive.
func descendants(pid int) []int {
	children := childrenByParent()
	var all []int
	queue := []int{pid}
	for len(queue) > 0 {
		next := queue[0]
		queue = queue[1:]
		for _, c := range children[next] {
			all = append(all, c)
			queue = append(queue, c)
		}
	}
	return all
}

// childrenByParent maps a process id to its children, read from /proc where there is one (Linux) and from
// ps elsewhere (macOS and the BSDs). When neither works the map is empty and only the group is killed.
func childrenByParent() map[int][]int {
	children := map[int][]int{}
	if entries, err := os.ReadDir("/proc"); err == nil {
		for _, e := range entries {
			pid, err := strconv.Atoi(e.Name())
			if err != nil {
				continue
			}
			data, err := os.ReadFile("/proc/" + e.Name() + "/stat")
			if err != nil {
				continue
			}
			// "pid (name) state ppid ...": the name may hold spaces and parentheses, so start after the last ")".
			s := string(data)
			fields := strings.Fields(s[strings.LastIndexByte(s, ')')+1:])
			if len(fields) < 2 {
				continue
			}
			if ppid, err := strconv.Atoi(fields[1]); err == nil {
				children[ppid] = append(children[ppid], pid)
			}
		}
		return children
	}
	out, err := exec.Command("ps", "-A", "-o", "pid=,ppid=").Output()
	if err != nil {
		return children
	}
	for _, line := range strings.Split(string(out), "\n") {
		f := strings.Fields(line)
		if len(f) != 2 {
			continue
		}
		pid, err1 := strconv.Atoi(f[0])
		ppid, err2 := strconv.Atoi(f[1])
		if err1 == nil && err2 == nil {
			children[ppid] = append(children[ppid], pid)
		}
	}
	return children
}
