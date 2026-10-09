# Changelog

All notable changes follow [Keep a Changelog](https://keepachangelog.com/) and this project uses
[Semantic Versioning](https://semver.org/). Until 1.0, anything may change between minor versions.

## [Unreleased]

## [0.1.0] - 2026-10-09

First public release. Early software: [docs/STATUS.md](docs/STATUS.md) says what has and has not been verified.

### Added

- Runs and workflows: a YAML list of agent, check and review steps, each run on its own git worktree and branch,
  with a report (`evidence.md`) written next to it.
- An account pool and scheduler. A usage limit saves the half-finished work, continues on another login, or waits
  for the reset, and the rest of the workflow still runs. Logins can ask for a specific model and effort.
- Independent review on another provider (or login), read-only, with anything the reviewer touches reverted.
- Codex and Claude Code backends. (Claude Code is untested against a real install.)
- Safety: secrets removed before anything is stored, a scrubbed environment for agents, a check that the agent's own
  sandbox can start, and an explicit `--no-sandbox`. One heavy check runs at a time across runs.
- A daemon (`legatus serve`) with a queue, resume after a restart, cancel and retry, and a web cockpit that listens
  only on the local computer and needs a secret key.
- GitHub: start a task from an issue, and open a pull request from a finished run, only when asked.
- Automations: schedules, and a watcher for labelled issues that only accepts listed authors.
- Commands: `run`, `serve`, `open`, `queue`, `pr`, `cancel`, `resume`, `runs [--status]`, `show`, `accounts`,
  `automations`, `clean`, `doctor`, `demo`, `version`.
