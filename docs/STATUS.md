# Status: what is verified, and what is not

Legatus is early (v0.1). This page exists so nobody has to guess. If something is not listed as verified, assume
it has not been run for real.

## Tested automatically

These run on every change, on Linux, macOS and Windows, with stand-in agents and real `git`:

- Runs and workflows; worktrees (twelve created at the same moment without lock errors).
- The account pool and scheduler; limit handling; waiting for a reset; checks and their feedback; independent
  review and its fallbacks; redaction and the scrubbed environment; cancel and resume; nine runs at once over three
  logins; one heavy check at a time across runs.
- The queue: worker limits, cancel, retry, resuming after a restart.
- The HTTP API and its protections (host name, origin, key, content policy) and live streams.
- Schedules and the GitHub watcher, with a fake clock: first sighting, catching up once, weekends, active limits,
  author checks, restarts, and hot reload of the file.
- Pull requests, against a scripted `gh` and a real local git remote (the push really happens; the arguments and
  the body are checked).
- The command line, including that the sample `automations.yaml` stays valid.

## Run for real

On one Windows PC:

- **Codex CLI 0.162** completed a small task end to end through Legatus using an existing login: branch, commit,
  check and report. The event shapes the parser relies on were captured from that CLI. Codex has also been used to
  work on Legatus itself, one task at a time, with the output reviewed by a person before merging
  ([how, and what went wrong](DOGFOOD.md)).
- Reading a GitHub issue through the real `gh` was checked once, read-only.
- The cockpit was used in a real browser (Chromium) against the daemon with stand-in agents: the run list, a run's
  live journal, report and diff, adding and switching logins, submitting a task from the form and watching it
  finish, the automations page, and a phone-sized screen.

## Not verified yet

- **Claude Code.** The backend is written from Claude Code's documented interface and tested against scripted
  output only. It has never been run against a real install.
- **A real usage-limit message.** The parser is tolerant of the wordings agents use, but it was written from
  documented and remembered messages, not captured from a real limit. If the reset time cannot be read, the login
  is set aside for 30 minutes.
- **A real second login and a real mid-task limit.**
- **A real pull request on GitHub.** The flow is tested, but nothing has been opened on a real repository.
- **macOS and Linux by hand.** The tests run there in CI; nothing has been driven by a person.
- **Firefox and Safari** for the cockpit.
- **Codex's own Windows sandbox** (see below).

## Sandboxing

Legatus is not an operating-system sandbox. It turns on the agent's own (Codex `workspace-write`, or `read-only`
for reviews) and checks that it can start before sending work.

On the Windows PC this was developed on, Codex's Windows sandbox could not start (`helper_unknown_error: setup
refresh had errors`, or "Access is denied"), and a model turn under it hangs instead of failing. So Legatus probes
first, stops in about a second with a clear message, and offers `--no-sandbox`. The agent then works in the run's
own worktree with a scrubbed environment, but it can reach anything your user account can. That choice is yours; it
is never the default. Whether the sandbox starts on other Windows machines is untested, and so is the path where it
does.

## Known limits

- The daemon listens only on the local computer. There is no login or encryption for reaching it from another
  device, so it is not for sharing or for a server yet.
- Only Codex and Claude Code are supported as agents.
- Agent limits are parsed from error text. A change in an agent's wording can make a limit look like an ordinary
  failure until the parser is updated.
