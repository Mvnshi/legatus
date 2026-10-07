package worktree

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
)

func mustGit(t *testing.T, dir string, args ...string) string {
	t.Helper()
	cmd := exec.Command("git", args...)
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %v\n%s", args, err, out)
	}
	return string(out)
}

func newRepo(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	mustGit(t, dir, "init", "-q", "-b", "main")
	mustGit(t, dir, "config", "user.name", "Tester")
	mustGit(t, dir, "config", "user.email", "tester@example.com")
	if err := os.WriteFile(filepath.Join(dir, "a.txt"), []byte("one\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	mustGit(t, dir, "add", "-A")
	mustGit(t, dir, "commit", "-q", "-m", "first")
	return dir
}

func TestCreateCommitAndDiff(t *testing.T) {
	repo := newRepo(t)
	m := New()
	ctx := context.Background()
	wt := filepath.Join(t.TempDir(), "w1")
	if err := m.Create(ctx, repo, "main", "legatus/one", wt); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(wt, "b.txt"), []byte("new\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	status, err := m.Status(ctx, wt)
	if err != nil || !strings.Contains(status, "b.txt") {
		t.Fatalf("status = %q, %v", status, err)
	}
	committed, err := m.CommitAll(ctx, wt, "legatus: implement")
	if err != nil || !committed {
		t.Fatalf("CommitAll = %v, %v", committed, err)
	}
	again, err := m.CommitAll(ctx, wt, "nothing to do")
	if err != nil || again {
		t.Fatalf("second CommitAll = %v, %v; want false, nil", again, err)
	}
	diff, err := m.Diff(ctx, wt, "main", 0)
	if err != nil || !strings.Contains(diff, "+new") {
		t.Fatalf("diff = %q, %v", diff, err)
	}
	// The original checkout is untouched.
	if _, err := os.Stat(filepath.Join(repo, "b.txt")); !os.IsNotExist(err) {
		t.Fatalf("the agent's file leaked into the original checkout: %v", err)
	}
	if err := m.Remove(ctx, repo, wt, "legatus/one", true); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(wt); !os.IsNotExist(err) {
		t.Fatalf("worktree still there: %v", err)
	}
}

// Cezar's open issue #1301: parallel runs fail worktree creation on a .git/config lock.
func TestManyWorktreesAtOnce(t *testing.T) {
	repo := newRepo(t)
	m := New()
	ctx := context.Background()
	root := t.TempDir()
	const n = 12
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			errs <- m.Create(ctx, repo, "main", fmt.Sprintf("legatus/r%d", i), filepath.Join(root, fmt.Sprintf("w%d", i)))
		}(i)
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("a parallel create failed: %v", err)
		}
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != n {
		t.Fatalf("%d worktrees, want %d", len(entries), n)
	}
}

func TestCreateRejectsAMissingBase(t *testing.T) {
	repo := newRepo(t)
	m := New()
	err := m.Create(context.Background(), repo, "no-such-branch", "legatus/x", filepath.Join(t.TempDir(), "w"))
	if err == nil || !strings.Contains(err.Error(), "no-such-branch") {
		t.Fatalf("err = %v", err)
	}
}

func TestCommitUsesAFallbackIdentityOnlyWhenNoneIsConfigured(t *testing.T) {
	repo := newRepo(t)
	m := New()
	ctx := context.Background()
	wt := filepath.Join(t.TempDir(), "w")
	if err := m.Create(ctx, repo, "main", "legatus/id", wt); err != nil {
		t.Fatal(err)
	}
	os.WriteFile(filepath.Join(wt, "c.txt"), []byte("x"), 0o600)
	if _, err := m.CommitAll(ctx, wt, "msg"); err != nil {
		t.Fatal(err)
	}
	author := strings.TrimSpace(mustGit(t, wt, "log", "-1", "--format=%an"))
	if author != "Tester" {
		t.Fatalf("author = %q, want the repository's configured Tester", author)
	}
}
