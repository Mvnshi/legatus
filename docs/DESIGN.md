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
internal/cli         commands: run, resume, runs, show, accounts, clean, doctor, demo
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

## Compared with Cezar

Facts below come from Cezar's README, site and public issues in October 2026; its code was not read.

| | Cezar | Legatus |
| --- | --- | --- |
| Language, install | TypeScript, `npx`, Node 20+ | Go, one binary |
| Windows | macOS, Linux, WSL2 listed | native |
| Agents | Claude Code, Codex, Copilot CLI, OpenCode, Cursor, Junie, Pi | Codex, Claude Code (more via the Backend interface) |
| Parallel runs in worktrees | yes (issue #1301: lock failure on parallel create) | yes; creation is serialised per repo, tested with 12 at once |
| Usage limits | auto-resume exists; #1300: remaining workflow steps not continued | login pool, continue on another login, wait for reset, finish the workflow |
| Cockpit, phone access | yes | not yet |
| GitHub / Jira / Linear, schedules | yes | not yet |
| Review of the result | PRs await a human | independent read-only agent review, then a human |
| Sandboxing | worktree isolation | worktree + agent sandbox checked first + scrubbed environment + redaction |
| Maturity | 1,400+ commits, 500+ stars | days old |

Where Cezar is ahead (cockpit, trackers, schedules, breadth of agents, maturity) the plan is to catch up on
the parts people use, not to copy its code. The wedge is the usage-limit, Windows, review and sandboxing rows.

## Roadmap

1. A daemon that owns the queue, so runs can be added while others run and survive restarts.
2. A web cockpit (live event stream, accounts and their limits, the report) served by that daemon.
3. Intake: GitHub issues and pull requests, then Jira and Linear; schedules.
4. Opening pull requests from a finished run, with the report as the description.
5. Usage probes (Codex reports rate limits) so the scheduler prefers the login with the most left.
6. A container sandbox, so Windows does not depend on the agent's own sandbox.
7. An installer and a signed release.
