# Changelog

All notable changes follow [Keep a Changelog](https://keepachangelog.com/) and this project uses
[Semantic Versioning](https://semver.org/). Until 1.0, anything may change between minor versions.

## [Unreleased]

### Fixed

- Claude Code's own wording for a usage limit (`You've hit your session limit · resets 3pm (America/New_York)`, and
  its weekly, model, spend and team-budget forms) was not recognised, and neither was its `rate_limit_event`. Both
  are now read, with the reset time taken from the event when there is one and from the message, in the time zone it
  names, when there is not.
- `Server is temporarily limiting requests (not your usage limit)` is no longer taken for a login's limit, so a
  working login is not set aside because the service is busy.
- Cancelling a run now also stops the commands the agent started in sessions of their own (Claude Code does this for
  its shell tool); they used to be left running.
- A review step that did not ask for another login or provider showed its independence as `()` in the run summary and
  in `evidence.md`; it now says `other-provider`, `other-account` or `same-account`.
- The cockpit's login-name field had a `pattern` that browsers reject, so they logged an error and skipped the check.
- `legatus doctor` no longer calls the Claude Code backend untested: it says which version was run for real.

### Changed

- The project website (`site/`) is rebuilt as a React, TypeScript and Vite app styled after Windows XP's Luna theme,
  served from `/legatus/`. It has a playable, clearly labelled "Simulated demo" of a usage limit being handled, real
  release information beside the install steps, and the verification limits from `docs/STATUS.md`. The Go program
  and the local cockpit are unchanged.

## [0.1.0] - 2026-10-09

First public release. Early software: [docs/STATUS.md](docs/STATUS.md) says what has and has not been verified.

### Added

- Runs and workflows: a YAML list of agent, check and review steps, each run on its own git worktree and branch,
  with a report (`evidence.md`) written next to it.
- An account pool and scheduler. A usage limit saves the half-finished work, continues on another login, or waits
  for the reset, and the rest of the workflow still runs. Logins can ask for a specific model and effort.
- Independent review on another provider (or login), read-only, with anything the reviewer touches reverted.
- Codex and Claude Code backends.
- Safety: secrets removed before anything is stored, a scrubbed environment for agents, a check that the agent's own
  sandbox can start, and an explicit `--no-sandbox`. One heavy check runs at a time across runs.
- A daemon (`legatus serve`) with a queue, resume after a restart, cancel and retry, and a web cockpit that listens
  only on the local computer and needs a secret key.
- GitHub: start a task from an issue, and open a pull request from a finished run, only when asked.
- Automations: schedules, and a watcher for labelled issues that only accepts listed authors.
- Commands: `run`, `serve`, `open`, `queue`, `pr`, `cancel`, `resume`, `runs [--status]`, `show`, `accounts`,
  `automations`, `clean`, `doctor`, `demo`, `version`.
