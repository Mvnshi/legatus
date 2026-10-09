# Backlog

Each item can be picked up as written. Sizes are rough: S is an hour or two, M a day, L more.

## Small

- **`legatus doctor` checks `gh`** (S). Whether it is installed and signed in (`gh auth status`), explained in the
  same style as the other lines, since pull requests and issue intake need it.
- **`legatus show` includes the changed files** (S). It prints the report but passes an empty diffstat. When the
  run's worktree still exists, include `git diff --stat` against the run's base commit.
- **`legatus merge <run>`** (S). Squash a finished run's branch into the current branch with a message written from
  the run's title and report, instead of the "legatus: implement" commits agents leave.
- **`--title` for `run` and `queue`** (S). The title is the first line of the task; long task texts make an ugly list.

## Medium

- **Stop a whole process tree on Windows with a Job Object** (M). Cancelling currently runs `taskkill /T`, which can
  take a long time on a busy machine. A job object with `KILL_ON_JOB_CLOSE` stops the tree atomically and needs no
  helper process. Needs a test that starts a child of a child and checks both are gone.
- **Usage probes** (M). Ask each login how much usage it has left (Codex reports rate limits) so the scheduler can
  prefer the login with the most left, and the cockpit can show a bar. `pool.SetUsage` already takes the number.
- **A cockpit view of issues the watcher ignored** (M). Show issues skipped because the author is not on the list,
  with a button that adds the author or dismisses it.
- **Jira and Linear intake** (M). The same shape as the GitHub watcher: a read-only client, an authors allow-list,
  issue text fenced off as untrusted.
- **Verify Claude Code beyond one version and one system** (M). It was run for real with 2.1.295 on Linux (see
  [STATUS.md](STATUS.md)). What remains is Windows, macOS, a personal subscription login, and a real usage limit
  from Anthropic rather than a simulated refusal.

## Larger

- **A container sandbox** (L). Run the agent in Docker or Podman with the worktree mounted, so Windows does not
  depend on the agent's own sandbox.
- **Safe access from another device** (L). Authenticated, encrypted access to the daemon for a server or a phone away
  from home. Not a flag that opens the port.
- **More agents** (L). A backend for another agent CLI, using the `agent.Backend` interface and the helper-process
  test pattern.
- **Signed releases and an installer** (L). Signed binaries per system, provenance attestations, and a one-line
  installer.
