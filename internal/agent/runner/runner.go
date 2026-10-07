// Package runner starts an agent program and streams what it prints, line by line. Backends use it so
// they all handle the same hard parts the same way: the prompt goes in through stdin (a diff can be far
// longer than a Windows command line allows), stderr is kept for error messages, and cancelling stops the
// whole process tree rather than just the first process.
package runner

import (
	"bufio"
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"strings"
	"sync"
	"time"
)

// Spec describes one process to run.
type Spec struct {
	Exe        string
	PrefixArgs []string // placed before Args; lets tests run themselves as the agent
	Args       []string
	Dir        string
	Env        []string
	Stdin      string
}

// Result is how the process ended.
type Result struct {
	ExitCode   int
	StderrTail string // the last few KB of stderr
}

const stderrKeep = 8 << 10

// tail keeps the last n bytes written to it.
type tail struct {
	mu  sync.Mutex
	buf []byte
	n   int
}

func (t *tail) Write(p []byte) (int, error) {
	t.mu.Lock()
	defer t.mu.Unlock()
	t.buf = append(t.buf, p...)
	if len(t.buf) > t.n {
		t.buf = t.buf[len(t.buf)-t.n:]
	}
	return len(p), nil
}

func (t *tail) String() string {
	t.mu.Lock()
	defer t.mu.Unlock()
	return string(t.buf)
}

// ErrNotFound means the agent program is not installed (or not on PATH).
type ErrNotFound struct{ Exe string }

func (e *ErrNotFound) Error() string {
	return fmt.Sprintf("%q was not found; install it and make sure it is on PATH", e.Exe)
}

// Stream runs the process and calls onLine for every line it prints on stdout. It returns after the
// process has exited and all its output has been read. A non-zero exit is not an error; the caller
// decides what it means. An error is returned only when the process could not run or ctx ended.
func Stream(ctx context.Context, s Spec, onLine func(line []byte)) (Result, error) {
	cmd := exec.CommandContext(ctx, s.Exe, append(append([]string{}, s.PrefixArgs...), s.Args...)...)
	cmd.Dir = s.Dir
	cmd.Env = s.Env
	cmd.Stdin = strings.NewReader(s.Stdin)
	configure(cmd)
	cmd.WaitDelay = 10 * time.Second
	stderr := &tail{n: stderrKeep}
	cmd.Stderr = stderr
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return Result{}, err
	}
	if err := cmd.Start(); err != nil {
		var nf *exec.Error
		if errors.As(err, &nf) {
			return Result{}, &ErrNotFound{Exe: s.Exe}
		}
		return Result{}, err
	}
	reader := bufio.NewReaderSize(stdout, 64<<10)
	for {
		line, err := reader.ReadBytes('\n')
		if len(line) > 0 {
			if trimmed := bytes.TrimRight(line, "\r\n"); len(trimmed) > 0 {
				onLine(trimmed)
			}
		}
		if err != nil {
			if !errors.Is(err, io.EOF) && ctx.Err() == nil {
				_ = cmd.Process.Kill()
			}
			break
		}
	}
	waitErr := cmd.Wait()
	if ctx.Err() != nil {
		return Result{StderrTail: stderr.String()}, ctx.Err()
	}
	res := Result{StderrTail: stderr.String()}
	var exit *exec.ExitError
	switch {
	case waitErr == nil:
	case errors.As(waitErr, &exit):
		res.ExitCode = exit.ExitCode()
	default:
		return res, waitErr
	}
	return res, nil
}
