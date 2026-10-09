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

On Linux (x64, Ubuntu 24.04, in a cloud container, October 2026):

- **Claude Code 2.1.295** ran through Legatus end to end: `legatus run` with an agent step, a check and a review step
  (branch, commit, report; the review was run by the same login, as only one was set up), a check that failed and
  sent the work back, and the daemon (`serve` and `queue`) working two logins at once, each with its own
  `CLAUDE_CONFIG_DIR`. What it printed is kept in `internal/agent/claude/testdata/` and read by the tests. All of
  these logins used this container's own credentials, not a personal subscription (see below).
- **Cancel** stops the agent and the shell command it started. Running it for real showed that the command was left
  running, because Claude Code starts commands in a session of its own; Legatus now also stops everything the agent
  started, and a test covers it.
- **A usage limit, with the real Claude Code on the receiving end.** A local stand-in for the Anthropic API refused a
  request the way an exhausted login is refused (HTTP 429 with the `anthropic-ratelimit-unified-*` headers). What
  Claude Code printed in answer is real: a `rate_limit_event` with status `rejected` and the exact reset time,
  `You've hit your session limit · resets 8:54pm (UTC)`, exit code 1. Legatus set that login aside until the exact
  time in the event and carried on with the second login, both when the refusal came at the first request and when
  it came mid-task, after the agent had run a command: the half-finished work was committed, the next login was told
  about it, and it finished the job. Only the server's refusal was simulated.
- The wording of Claude Code's other limit messages (a weekly or model limit, a monthly spend limit, a team budget,
  the service throttling everyone, which is not a login limit) was read from the program itself rather than provoked.
  Two bugs came out of that: the real wording `You've hit your session limit` was not recognised as a limit, and the
  throttle message `Server is temporarily limiting requests (not your usage limit)` was. The reset time Claude Code
  prints names its time zone (`resets 3pm (America/New_York)`), which was being read in the wrong one.
- **`legatus pr --draft`** pushed a run's branch to a real GitHub repository. Its next step, `gh pr create`, was refused by
  this container's GitHub proxy (it blocks GraphQL, which `gh pr create` uses), and Legatus said so in plain words. The
  pull request was then opened from the pushed branch with a different tool, using the title and report Legatus made.
- The cockpit (`legatus serve --demo`) was driven in Chromium 141, Firefox 157 and the WebKit 27.2 build that
  Playwright ships for Linux: opening it with its key, submitting a task, the live journal, report and diff, the
  filter, adding a login, the automations page, and a phone-sized screen. This found one bug (the login-name field's
  `pattern` was not a valid regular expression, so browsers logged an error and checked nothing). WebKit also logs a
  content-policy message for every `<select>`; it reproduces on a page with no Legatus code and changes nothing on
  screen.
- The project site's browser tests pass in Firefox (all but two that need WebGL, which headless Firefox here lacks and
  which skip) and in that WebKit build.

## Not verified yet

- **A real usage limit.** Claude Code was run against a refusal simulated by a local server, because a login cannot be
  run out of usage on demand: how it reacts is real, what the Anthropic service sends for a real limit is not
  captured. Codex's limit messages were written from documented and remembered wordings and never captured. If the
  reset time cannot be read, the login is set aside for 30 minutes.
- **A personal subscription login for Claude Code** (`legatus accounts login`, or a login kept in the system keychain).
  The runs above used the credentials this container provides, with a separate configuration folder per login.
- **A real pull request opened by Legatus.** The push to a real repository worked. `gh pr create` has never succeeded
  against one: the container used for the runs above could not run it, and the rest of the flow is tested against a
  scripted `gh`.
- **macOS by hand, and Claude Code on Windows and macOS.** The tests run there in CI; nothing has been driven by a
  person. Codex was run for real only on Windows.
- **Safari itself.** The cockpit and the site ran in Firefox and in Playwright's WebKit build on Linux, not in Apple's
  Safari.
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
- Agent limits are read from the agent's output: for Claude Code its `rate_limit_event`, then its message. A change
  in an agent's wording can make a limit look like an ordinary failure until the parser is updated. Only Claude Code
  2.1.295 was run for real; `legatus doctor` says so for any other version.
