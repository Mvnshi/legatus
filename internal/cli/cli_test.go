package cli

import (
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// invoke runs the command line with its own empty Legatus home.
func invoke(t *testing.T, home string, args ...string) (code int, stdout, stderr string) {
	t.Helper()
	t.Setenv("LEGATUS_HOME", home)
	var out, errb bytes.Buffer
	code = Main(args, &out, &errb)
	return code, out.String(), errb.String()
}

func TestHelpAndUnknownCommands(t *testing.T) {
	home := t.TempDir()
	if code, out, _ := invoke(t, home); code != 0 || !strings.Contains(out, "legatus run") {
		t.Fatalf("help: %d %q", code, out)
	}
	if code, _, errText := invoke(t, home, "frobnicate"); code != 64 || !strings.Contains(errText, `unknown command "frobnicate"`) {
		t.Fatalf("unknown: %d %q", code, errText)
	}
	if code, out, _ := invoke(t, home, "version"); code != 0 || !strings.HasPrefix(out, "legatus ") {
		t.Fatalf("version: %d %q", code, out)
	}
}

func TestTheDemoShowsALimitBeingHandled(t *testing.T) {
	code, out, errText := invoke(t, t.TempDir(), "demo")
	if code != 0 {
		t.Fatalf("exit %d\n%s\n%s", code, out, errText)
	}
	for _, want := range []string{
		"work-1 hit its usage limit", "Continuing on another login", "working as work-2",
		"independent: other-provider", "run succeeded", "(interrupted by a usage limit)",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("demo output lacks %q", want)
		}
	}
}

func TestAccountsLifecycle(t *testing.T) {
	home := t.TempDir()
	if code, out, _ := invoke(t, home, "accounts", "ls"); code != 0 || !strings.Contains(out, "No accounts yet") {
		t.Fatalf("empty ls: %d %q", code, out)
	}
	if code, out, errText := invoke(t, home, "accounts", "add", "--id", "main", "--provider", "codex", "--home", "default"); code != 0 || !strings.Contains(out, "Added main") {
		t.Fatalf("add: %d %q %q", code, out, errText)
	}
	if code, out, _ := invoke(t, home, "accounts", "add", "--id", "second", "--provider", "claude", "--max", "2"); code != 0 || !strings.Contains(out, "legatus accounts login second") {
		t.Fatalf("add second: %d %q", code, out)
	}
	if _, err := os.Stat(filepath.Join(home, "accounts", "claude-second")); err != nil {
		t.Fatalf("the new login's folder was not created: %v", err)
	}
	code, out, _ := invoke(t, home, "accounts", "ls")
	if code != 0 || !strings.Contains(out, "main") || !strings.Contains(out, "second") || !strings.Contains(out, "ready") || !strings.Contains(out, "0/2") {
		t.Fatalf("ls: %d\n%s", code, out)
	}
	if code, _, errText := invoke(t, home, "accounts", "add", "--id", "main", "--provider", "codex"); code != 1 || !strings.Contains(errText, "already exists") {
		t.Fatalf("duplicate: %d %q", code, errText)
	}
	if code, _, _ := invoke(t, home, "accounts", "disable", "second"); code != 0 {
		t.Fatalf("disable: %d", code)
	}
	if _, out, _ := invoke(t, home, "accounts", "ls"); !strings.Contains(out, "disabled") {
		t.Fatalf("disabled state not shown:\n%s", out)
	}
	if code, _, _ := invoke(t, home, "accounts", "rm", "second"); code != 0 {
		t.Fatalf("rm: %d", code)
	}
	if _, out, _ := invoke(t, home, "accounts", "ls"); strings.Contains(out, "second") {
		t.Fatalf("removed account still listed:\n%s", out)
	}
	if code, _, errText := invoke(t, home, "accounts", "rm", "ghost"); code != 1 || !strings.Contains(errText, "no account named") {
		t.Fatalf("rm ghost: %d %q", code, errText)
	}
}

func TestAccountInputsAreValidated(t *testing.T) {
	home := t.TempDir()
	for _, args := range [][]string{
		{"accounts", "add", "--id", "Bad Name", "--provider", "codex"},
		{"accounts", "add", "--id", "ok", "--provider", "gemini"},
		{"accounts", "add", "--id", "ok", "--provider", "codex", "--max", "0"},
		{"accounts"},
		{"accounts", "bogus"},
	} {
		if code, _, _ := invoke(t, home, args...); code != 64 {
			t.Errorf("%v: exit %d, want 64", args, code)
		}
	}
}

func TestRunNeedsATaskAndAnAccount(t *testing.T) {
	home := t.TempDir()
	if code, _, errText := invoke(t, home, "run"); code != 64 || !strings.Contains(errText, "say what to do") {
		t.Fatalf("no task: %d %q", code, errText)
	}
	if code, _, errText := invoke(t, home, "run", "do a thing"); code != 1 || !strings.Contains(errText, "no accounts yet") {
		t.Fatalf("no accounts: %d %q", code, errText)
	}
}

func TestRunsShowAndCleanAfterTheDemoKeepsItsFiles(t *testing.T) {
	// The demo uses its own temporary home, so build a run with the stand-in agents in this one.
	home := t.TempDir()
	repo := filepath.Join(t.TempDir(), "repo")
	if err := makeDemoRepo(repo); err != nil {
		t.Fatal(err)
	}
	if code, _, errText := invoke(t, home, "runs"); code != 0 || errText != "" {
		t.Fatalf("runs on an empty home: %d %q", code, errText)
	}
	if code, _, _ := invoke(t, home, "show", "nope"); code != 1 {
		t.Fatalf("show of an unknown run should fail, got %d", code)
	}
	if code, out, _ := invoke(t, home, "clean"); code != 0 || !strings.Contains(out, "Nothing to clean") {
		t.Fatalf("clean: %d %q", code, out)
	}
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git is not installed")
	}
}
