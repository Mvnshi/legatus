// Package worktree gives every run its own git worktree and branch, so many agents can work on one
// repository at once without touching each other's files.
package worktree

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// Manager serialises the git operations that take a repository-wide lock. Running several `git worktree
// add` at the same moment otherwise fails with "could not lock config file".
type Manager struct {
	Git string // the git executable; "git" when empty

	mu    sync.Mutex
	repos map[string]*sync.Mutex
}

// New returns a Manager using the git on PATH.
func New() *Manager { return &Manager{repos: map[string]*sync.Mutex{}} }

func (m *Manager) git() string {
	if m.Git != "" {
		return m.Git
	}
	return "git"
}

func (m *Manager) repoLock(repo string) *sync.Mutex {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.repos == nil {
		m.repos = map[string]*sync.Mutex{}
	}
	key := strings.ToLower(filepath.Clean(repo))
	l, ok := m.repos[key]
	if !ok {
		l = &sync.Mutex{}
		m.repos[key] = l
	}
	return l
}

// Error is a failed git command with its output.
type Error struct {
	Args   []string
	Stderr string
	Err    error
}

func (e *Error) Error() string {
	return fmt.Sprintf("git %s: %v: %s", strings.Join(e.Args, " "), e.Err, strings.TrimSpace(e.Stderr))
}
func (e *Error) Unwrap() error { return e.Err }

func lockContention(stderr string) bool {
	s := strings.ToLower(stderr)
	return strings.Contains(s, "could not lock") || strings.Contains(s, ".lock': file exists") ||
		strings.Contains(s, "unable to create") && strings.Contains(s, ".lock")
}

// run executes git in dir and retries a few times when another git process holds a lock.
func (m *Manager) run(ctx context.Context, dir string, args ...string) (string, error) {
	var lastErr error
	for attempt := 0; attempt < 8; attempt++ {
		cmd := exec.CommandContext(ctx, m.git(), args...)
		cmd.Dir = dir
		var stdout, stderr bytes.Buffer
		cmd.Stdout, cmd.Stderr = &stdout, &stderr
		cmd.Env = append(os.Environ(), "GIT_TERMINAL_PROMPT=0", "LC_ALL=C")
		err := cmd.Run()
		if err == nil {
			return stdout.String(), nil
		}
		lastErr = &Error{Args: args, Stderr: stderr.String(), Err: err}
		if ctx.Err() != nil || !lockContention(stderr.String()) {
			return stdout.String(), lastErr
		}
		select {
		case <-ctx.Done():
			return stdout.String(), ctx.Err()
		case <-time.After(time.Duration(attempt+1) * 150 * time.Millisecond):
		}
	}
	return "", lastErr
}

// Resolve turns a branch, tag or commit name into the commit it points at right now.
func (m *Manager) Resolve(ctx context.Context, repo, ref string) (string, error) {
	repo, err := filepath.Abs(repo)
	if err != nil {
		return "", err
	}
	if ref == "" {
		ref = "HEAD"
	}
	out, err := m.run(ctx, repo, "rev-parse", "--verify", "--quiet", ref+"^{commit}")
	if err != nil {
		return "", fmt.Errorf("%s is not a git repository with a commit named %q", repo, ref)
	}
	return strings.TrimSpace(out), nil
}

// Create adds a worktree at dir on a new branch started from base.
func (m *Manager) Create(ctx context.Context, repo, base, branch, dir string) error {
	repo, err := filepath.Abs(repo)
	if err != nil {
		return err
	}
	if base == "" {
		base = "HEAD"
	}
	if _, err := m.run(ctx, repo, "rev-parse", "--verify", "--quiet", base+"^{commit}"); err != nil {
		return fmt.Errorf("%s is not a git repository with a commit named %q", repo, base)
	}
	if err := os.MkdirAll(filepath.Dir(dir), 0o700); err != nil {
		return err
	}
	l := m.repoLock(repo)
	l.Lock()
	defer l.Unlock()
	_, err = m.run(ctx, repo, "worktree", "add", "-b", branch, dir, base)
	return err
}

// Remove deletes the worktree and, when asked, its branch.
func (m *Manager) Remove(ctx context.Context, repo, dir, branch string, deleteBranch bool) error {
	l := m.repoLock(repo)
	l.Lock()
	defer l.Unlock()
	_, err := m.run(ctx, repo, "worktree", "remove", "--force", dir)
	if err != nil {
		var ge *Error
		if errors.As(err, &ge) && strings.Contains(ge.Stderr, "is not a working tree") {
			err = nil
		}
	}
	if err == nil && deleteBranch && branch != "" {
		_, err = m.run(ctx, repo, "branch", "-D", branch)
	}
	return err
}

// Status is `git status --short`: empty when the worktree is clean.
func (m *Manager) Status(ctx context.Context, dir string) (string, error) {
	return m.run(ctx, dir, "status", "--short")
}

// DiffStat summarises uncommitted and committed changes against base.
func (m *Manager) DiffStat(ctx context.Context, dir, base string) (string, error) {
	if base == "" {
		base = "HEAD"
	}
	if _, err := m.run(ctx, dir, "add", "-N", "."); err != nil {
		return "", err
	}
	return m.run(ctx, dir, "diff", "--stat", base)
}

// Diff is the full patch against base, cut at maxBytes (0 means no limit).
func (m *Manager) Diff(ctx context.Context, dir, base string, maxBytes int) (string, error) {
	if base == "" {
		base = "HEAD"
	}
	if _, err := m.run(ctx, dir, "add", "-N", "."); err != nil {
		return "", err
	}
	out, err := m.run(ctx, dir, "diff", base)
	if err != nil {
		return "", err
	}
	if maxBytes > 0 && len(out) > maxBytes {
		out = out[:maxBytes] + "\n... (diff cut)\n"
	}
	return out, nil
}

// Patch is the patch of what is committed or tracked-and-changed against base, cut at maxBytes. Unlike Diff
// it only reads: it does not touch the index, so it is safe to call while an agent is working in the
// worktree.
func (m *Manager) Patch(ctx context.Context, dir, base string, maxBytes int) (string, error) {
	if base == "" {
		base = "HEAD"
	}
	if _, err := os.Stat(dir); err != nil {
		return "", err
	}
	out, err := m.run(ctx, dir, "diff", "--no-ext-diff", "--stat", "--patch", base)
	if err != nil {
		return "", err
	}
	if maxBytes > 0 && len(out) > maxBytes {
		out = out[:maxBytes] + "\n... (cut)\n"
	}
	return out, nil
}

// CommitAll commits every change in the worktree, including files that check steps wrote. It reports
// whether there was anything to commit. It never changes the repository's git configuration.
func (m *Manager) CommitAll(ctx context.Context, dir, message string) (bool, error) {
	if _, err := m.run(ctx, dir, "add", "-A"); err != nil {
		return false, err
	}
	status, err := m.run(ctx, dir, "status", "--porcelain")
	if err != nil {
		return false, err
	}
	if strings.TrimSpace(status) == "" {
		return false, nil
	}
	args := []string{}
	if name, _ := m.run(ctx, dir, "config", "user.name"); strings.TrimSpace(name) == "" {
		args = append(args, "-c", "user.name=Legatus")
	}
	if email, _ := m.run(ctx, dir, "config", "user.email"); strings.TrimSpace(email) == "" {
		args = append(args, "-c", "user.email=legatus@localhost")
	}
	args = append(args, "commit", "--no-verify", "-m", message)
	_, err = m.run(ctx, dir, args...)
	return err == nil, err
}

// Reset throws away every uncommitted change, including new files, leaving the worktree at its last
// commit. It is used to undo what a reviewer touched.
func (m *Manager) Reset(ctx context.Context, dir string) error {
	if _, err := m.run(ctx, dir, "reset", "--hard", "HEAD"); err != nil {
		return err
	}
	_, err := m.run(ctx, dir, "clean", "-fdq")
	return err
}

// Head is the commit the worktree is at.
func (m *Manager) Head(ctx context.Context, dir string) (string, error) {
	out, err := m.run(ctx, dir, "rev-parse", "HEAD")
	return strings.TrimSpace(out), err
}
