# Backlog

Each item can be handed to an agent as written. Sizes are rough: S is an hour of an agent's time, M a few hours.

## Small

- **Tests for `internal/agent/runner`** (S). It has no tests. Cover: every stdout line reaches the callback; the
  stderr tail is kept and cut to its limit; a non-zero exit is reported in the result, not as an error; a program
  that does not exist gives `*ErrNotFound`; a very long prompt arrives intact on stdin; cancelling stops the
  process (and on Windows and Unix, its children) promptly. Use the helper-process pattern from
  `internal/agent/codex/codex_test.go`.
- **`legatus show` includes the changed files** (S). It prints the report but passes an empty diffstat. When the
  run's worktree still exists, include `git diff --stat` against the run's base commit.
- **`legatus runs --status`** (S). Filter the list by status (`failed`, `needs_human`, ...); reject unknown ones.
- **`legatus doctor` checks `gh`** (S). Whether it is installed and signed in (`gh auth status`), explained in the
  same style as the other lines, since pull requests and issue intake need it.
- **A real version number** (S). `legatus version` prints `dev`. Set it at build time with `-ldflags -X`, and fall
  back to the commit recorded by `runtime/debug.ReadBuildInfo`.

## Medium

- **Usage probes** (M). Ask each login how much usage it has left (Codex reports rate limits) so the scheduler
  can prefer the login with the most left, and the cockpit can show a bar. `pool.SetUsage` already takes the number.
- **A cockpit page for the GitHub watcher's ignored issues** (M). Show issues that were skipped because the author
  is not on the list, with a button that adds the author or ignores it.
- **Jira and Linear intake** (M). The same shape as the GitHub watcher: a read-only client, an authors
  allow-list, issues fenced off as untrusted text.

## Larger

- **A container sandbox** (L). Run the agent in Docker or Podman with the worktree mounted, so Windows does not
  depend on the agent's own sandbox.
- **Safe access from another device** (L). Authenticated, encrypted access to the daemon for a VPS or a phone away
  from home. Not a flag that opens the port.
- **Installer and release** (L). One download per system, signed, with a one-line installer.
