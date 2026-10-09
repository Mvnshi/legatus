# Legatus

Run coding agents (Codex, Claude Code) in parallel on your own machine, spread across **your own**
subscriptions, and keep going when one of them hits its usage limit.

```text
legatus run --repo . --check "go test ./..." --review "fix the failing login test"
```

Each task gets its own git worktree and branch. Legatus picks a login that still has usage, runs the agent,
runs your checks, has a *different* agent review the change, and leaves a branch plus a report you can read
before you merge. If a login runs out of usage halfway, the half-finished work is committed, the next login
is told exactly where things stand, and the rest of the workflow still runs. If every login is out, the run
waits for the first reset and carries on.

> **Status: early.** The core works and is tested; some parts have only been exercised with stand-in agents.
> [What is verified, and what is not](#what-is-verified) is spelled out below. Nothing here is released yet.

## Try it without installing an agent

```text
legatus demo
```

Two stand-in Codex logins and one stand-in Claude login. The first Codex login runs out of usage halfway
through the task; you can watch the work move to the second, the check run, and the Claude login review it.

## Use it for real

You need `git` and at least one agent CLI that is signed in: the [Codex CLI](https://github.com/openai/codex)
or Claude Code. Then:

```text
legatus doctor                                              # what is installed, which logins are found
legatus accounts add --id main --provider codex --home default   # use the login Codex already has
legatus accounts add --id second --provider codex                # a second login, in its own folder
legatus accounts login second                                    # you sign in; Legatus never sees a password
legatus run --repo . --check "npm test" "add input validation to the signup form"
```

Useful flags: `--review` (independent review), `--agent codex|claude|any`, `--workflow file.yaml`,
`--no-sandbox` (see [Sandboxing](#sandboxing)). `legatus runs [--status <status>]` (for example, `--status failed`
or `--status needs_human`), `legatus show <run>`, `legatus resume <run>`,
`legatus clean`.

## From a GitHub issue to a pull request

```text
legatus run --issue Mvnshi/legatus#12 --review --pr draft --check "go test ./..."
legatus pr <run>             # or open the pull request later, after you have looked at the branch
```

`--issue` reads the issue through your signed-in `gh` and gives it to the agent as the problem to solve; the
issue text is fenced off and the agent is told not to take orders from it, because anyone can write an issue.
`--pr` pushes the run's branch to `origin` and opens the pull request with the run's report as its description
(and `Closes owner/repo#12` for an issue). Nothing is pushed or published unless you ask for it for that run,
either with `--pr`, with `legatus pr`, or with the button in the cockpit, which asks you to confirm. A run that
needs a person (the reviewer disagreed) only ever opens as a draft.

## Automations: tasks that start by themselves

With the daemon running, standing instructions in `automations.yaml` start runs for you: on a schedule, or when
an issue with a label appears.

```text
legatus automations example      # a sample file to start from
legatus automations check        # validate it (the daemon also picks up edits within a minute)
```

```yaml
automations:
  - id: weekly-deps
    schedule: weekly mon 09:00      # every 6h | daily 09:00 | weekdays 09:00 | weekly mon 09:00
    repo: C:/code/my-app
    prompt: Update the dependencies, fix whatever breaks, and keep the tests passing.
    checks: ["npm test"]
    review: true
    pr: draft
    max_active: 1                   # do not start another while one is still going
  - id: labelled-issues
    github: {repo: you/my-app, label: legatus, every: 10m, authors: [you]}
    repo: C:/code/my-app
    checks: ["npm test"]
    pr: draft
```

A schedule first runs at the next scheduled time after you add it, never straight away, and a missed time is run
once, not repeatedly. A watch takes each issue once. **A watch only accepts issues from the logins you list under
`authors`** (the file is refused without them): anyone who could write or label an issue could otherwise steer
an agent on your computer. `anyone: true` turns that check off and is yours to justify. The cockpit's
Automations page shows what is on, when it last ran and runs next, and has a "Run now" button.

## The daemon and the cockpit

`legatus run` works on one task in your terminal. To queue many, leave Legatus running:

```text
legatus serve                 # the daemon: a queue, several runs at once, survives restarts
legatus open                  # the web cockpit, in your browser
legatus queue "add input validation to the signup form" --check "npm test" --review
legatus serve --demo          # try the cockpit with stand-in agents and nothing installed
```

The cockpit shows every run with its steps and which login did what, a live journal, the report, the diff,
and your logins with their limits and a countdown to each reset. You can start, cancel and retry tasks and
add logins from it, and it works on a phone-sized screen. Runs that were not finished when the daemon stopped
continue the next time it starts.

It listens only on this computer. Every request needs a secret key kept in a file only you can read, and
requests that name any other host are refused (so a web page cannot reach it). Whoever has the key can have
agents work in any repository on this machine, so it is not for sharing. Reaching it from another device needs
a tunnel you set up yourself; Legatus has no login or encryption for that yet.

A run's branch is `legatus/<run id>`. The report is `evidence.md` in the run's folder; it lists which login did
what, the check results, the reviewer's verdict, every usage limit that was hit, anything removed from prompts,
and the changed files.

## Workflows

```yaml
name: ship
steps:
  - id: implement
    type: agent
    agent: any                     # any, a provider (codex, claude), or other-than:<step>
  - id: checks
    type: check
    run: ["go test ./...", "go vet ./..."]
    on_fail: {retry_with: implement, max: 2}     # send the failure back to the agent
  - id: review
    type: review
    agent: other-than:implement    # another provider if you have one, else another login
    on_fail: {retry_with: implement, max: 1}
```

## What makes it different

Cezar ([open-mercato/cezar](https://github.com/open-mercato/cezar), MIT) is the closest project and does a lot
well: a cockpit, trackers, schedules. Legatus is built around four things where Cezar looks weaker or says
nothing, judging only by its public README, site and issue list in October 2026 (its code was not read, and it
moves fast, so check before quoting this):

| | |
| --- | --- |
| **A queue that survives usage limits** | A pool of your own logins; work goes to one with usage left; a limit mid-task continues on another login *and finishes the remaining workflow steps*, or waits for the reset. |
| **Native Windows** | One Go binary. No Node, no WSL2. |
| **Safe by default** | Secrets are removed from prompts before anything is stored or sent; agents get a scrubbed environment; the agent's own sandbox is checked first and Legatus refuses to run without it unless you say so. |
| **Independent review** | A different provider (or at least a different login) reviews read-only; whatever a reviewer touches is reverted. No verdict is never an approval. |

## What is verified

Tested automatically (stand-in agents, real git): runs and workflows, worktrees (12 created at once without
lock errors), the account pool and scheduler, limit handling and waiting for a reset, check feedback,
independent review and its fallbacks, redaction and environment scrubbing, cancel and resume, nine runs at
once over three logins, schedules and the GitHub watcher (with a fake clock: first sighting, catching up once,
weekends, active limits, author checks, restarts, hot reload of the file), the queue (worker limits, cancel, retry, resuming after a restart), the HTTP API and
its security checks (host, origin, key, content policy), live streams, the command line.

The cockpit was used in a real browser (Chromium, Windows) against the daemon with stand-in agents: the run
list, a run's live journal, report and diff, adding and switching logins, submitting a task from the form and
watching it finish, and a phone-sized screen. It has not been tried on Firefox or Safari.

Pull requests were tested with a scripted `gh` and a real local git remote (the push really happens; the
`gh pr create` arguments and body are checked). Reading an issue was also checked once against real GitHub
through the real `gh` (read-only). **No pull request has been opened on real GitHub yet.**

Run for real on one Windows PC: **Codex CLI 0.162** completed a small task end to end through Legatus using an
existing login (`--no-sandbox`), producing the branch, commit, check and report. The Codex event shapes the
parser relies on were captured from that CLI.

**Not verified yet:**

- The Claude backend has never been run against a real Claude Code install (none was available).
- Usage-limit wording: the parser is tolerant, but it was written from documented and remembered messages, not
  captured from a real limit. If the reset time cannot be read, the login is set aside for 30 minutes.
- A real second login and a real mid-task limit.
- macOS and Linux (the code is portable and CI builds it there; nothing has been run there by hand).
- Codex's own Windows sandbox: on the machine this was written on it fails to start (see below), so the
  "sandbox works" path is untested.

## Sandboxing

Legatus is not an operating-system sandbox. It turns on the agent's own (Codex `workspace-write`, or
`read-only` for reviews) and checks that it can start before sending work. On the Windows PC this was built
on, Codex's Windows sandbox could not start (`helper_unknown_error: setup refresh had errors`), and a model
turn under it hangs rather than failing. So Legatus probes first, fails in about a second with a clear message,
and offers `--no-sandbox`: the agent then works in the run's own worktree with a scrubbed environment but can
reach anything your user account can. That is a choice for you to make; it is never the default.

## Use only your own logins

Spread work across subscriptions you are entitled to use, within each provider's terms. Legatus does not share
or pool anyone else's account, and it never asks for or stores a password: each login lives in its own folder
and you sign in with the agent's own `login` command.

## Roadmap

Not built yet: Jira and Linear intake, a container sandbox, usage probes
so the scheduler prefers the login with the most left, reaching the daemon safely from another device, an
installer. See [docs/DESIGN.md](docs/DESIGN.md).

## Develop

```text
go vet ./... && go test ./...
go build -o legatus ./cmd/legatus
```

MIT licensed.
