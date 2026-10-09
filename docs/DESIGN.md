# Design

## The idea in one paragraph

Coding agents are rate-limited by subscription, and a person who pays for more than one login has capacity
that no tool uses well. Legatus treats logins as a pool. A run is a YAML workflow of steps; every agent call
leases one login; a usage limit benches that login until it resets and the *same workflow continues* on
another. Everything a person needs to trust the result (which login did what, check output, an independent
review, secrets that were removed) is written to a report next to the branch.

## Parts

```text
cmd/legatus          the program
internal/cli         commands: run, serve, open, queue, resume, runs, show, accounts, clean, doctor, demo
internal/queue       the line of runs, workers, cancel, retry, resume after a restart
internal/hub         "something changed" notifications for live views
internal/server      JSON API, event streams and the embedded web cockpit (server/ui)
internal/app         where files live, saved accounts, wiring
internal/engine      walks a run through its workflow (steps, limits, review, evidence)
internal/pool        accounts, leases, limits, scheduling
internal/agent       the Backend interface, limit-message parsing
  agent/codex        codex exec --json
  agent/claude       claude -p --output-format stream-json   (unverified against a real install)
  agent/runner       starts a process, streams stdout, kills the whole tree on cancel
  agent/fake         scriptable stand-in, used by tests and `legatus demo`
internal/workflow    the YAML format and its validation
internal/worktree    one git worktree + branch per run, serialised against lock errors
internal/sandbox     secret redaction and the scrubbed environment
internal/store       runs as plain files: run.json, events.jsonl, evidence.md, workflow.yaml
internal/model       shared data types
```

State is plain files under the Legatus home (`%APPDATA%\legatus`, `~/.config/legatus`, or `$LEGATUS_HOME`).
There is no database. One process owns the directory at a time.

## Decisions worth knowing

**A login is a configuration directory.** Codex reads `CODEX_HOME`, Claude Code `CLAUDE_CONFIG_DIR`. A pool of
logins is therefore a pool of folders, each signed in with the agent's own `login`. Legatus never touches
credentials. `--home default` reuses the login the agent already has.

**A limit continues the work; it does not move a session.** Agent sessions cannot be handed to another login,
so the interrupted attempt is committed, its changed files are summarised into the next prompt, and the next
login continues from the worktree. Work is never thrown away, at the cost of a fresh session.

**Waiting is a state, not a goroutine.** When every matching login is limited, the pool returns the earliest
reset and the run is saved as `waiting_capacity` with a `wait_until`. `Execute` is resumable at every point:
cancelling (Ctrl-C, a crash) leaves the run in `running` or `waiting_capacity`, and `resume` continues it.

**Reviews are independent in three tiers**, recorded in the report: a different provider; else a different
login of the same provider; else the author's own login (clearly labelled, never silent). A review runs
read-only, and whatever it changes is reverted. A reply without a parseable verdict hands the run to a person.

**Nothing is a success by exit code alone.** A step that changes no files fails (unless `allow_empty`): in
testing, Codex exited cleanly after saying it could not write the file. A check or review that keeps failing
ends the run as failed or `needs_human`, never as success.

**Redaction happens before storage.** The task text is redacted when the run is created, so the run file, the
journal and the report (which is meant for pull requests) only ever hold redacted text. The agent's prompt is
redacted again. Findings record the kind and count, never the value.

**The agent's sandbox is checked, not assumed.** See the README. `Preflighter` backends are probed once per
login per process; a failed probe fails the run fast with instructions. `--no-sandbox` is explicit and noisy.

## The daemon

`legatus serve` owns the Legatus directory. `internal/queue` keeps a line of waiting runs and a fixed number of
workers (default 6; the account pool is the real limiter on agent work). On start it resumes every run that was
queued, running or waiting for capacity, oldest first. A run's worktree is created when it starts, not when it
is queued, and its base commit is pinned when it is submitted, so a task means the same commit however long it
waits. Cancel removes a queued run at once or interrupts a working one; Retry puts a failed, cancelled or
needs-a-person run back in line from the step that stopped it, keeping the work already on its branch.

`internal/hub` carries no data, only "something changed": a listener takes a channel, reads the journal on
disk, and waits on the channel. The journal is the single source of truth, so live views cannot miss or repeat
an event. The report is written before the final status is saved, so nothing shows a finished run without it.

`internal/server` is the JSON API, two server-sent-event streams and the cockpit (plain JavaScript, no build
step, embedded in the binary). It is loopback-only by design: requests must name a loopback host (against DNS
rebinding), must not come from another origin, and must carry the key from `<home>/token`. The cockpit's
content policy allows only its own scripts and styles, and it builds every element with DOM calls, never from
strings, because agent output is untrusted text. The key reaches the page in the URL fragment, which browsers
do not send to servers.

Two things only showed up under concurrency and are fixed: on Windows, replacing `run.json` fails with "access
denied" for a moment while anything is reading it (the store retries), and a Retry that arrives while the
worker is still finishing the run is queued behind it rather than dropped.

## GitHub

Everything goes through the user's own `gh`, so Legatus never holds a token. An issue becomes a task whose
prompt fences the issue text off as untrusted (issues are written by strangers). Publishing is explicit and
per run: `OpenPullRequest` pushes the branch to `origin`, opens the pull request with `evidence.md` as its
description, records the address on the run, and is idempotent (a second call, or a branch that already has a
pull request, returns the existing one). A run that needs a person is opened as a draft and never claims to
close an issue.

## Automations

`internal/automation` reads `automations.yaml` (hot-reloaded; a broken edit keeps the last good version and says
so) and ticks every 30 seconds. A schedule counts from the first time it is seen, so enabling "daily 09:00" at
three in the afternoon does not start a run at once; a time that was missed is owed once, not repeatedly; and
nothing new starts while `max_active` runs of the same automation are still going. A GitHub watch looks at
labelled issues on its own interval and takes each issue once, remembered across restarts. Because an issue can
be written by anyone, a watch needs an explicit list of trusted authors, and the author is checked again on the
full issue before anything starts.

## Roadmap

1. More intake: Jira and Linear.
2. Usage probes (Codex reports rate limits) so the scheduler prefers the login with the most left.
3. A container sandbox, so Windows does not depend on the agent's own sandbox.
4. Safe access from another device (authenticated, encrypted), for a VPS or a phone away from home.
5. An installer and a signed release.
