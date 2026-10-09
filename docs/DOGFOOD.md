# Legatus builds Legatus

The project is developed with itself. That is the test that matters: if the tool cannot handle its own
maintenance, it is not ready for anyone else's.

## How

Work is described as a small, checkable task, queued on the daemon, done by an agent in its own worktree, checked
by the repository's own checks, reviewed by an independent agent, and merged by a person who has read the diff.

```text
legatus serve                                   # leave it running
legatus queue --repo C:/code/legatus --agent codex --review \
  --check "go vet ./..." --check "go test ./..." \
  "Add tests for internal/agent/runner ..."
legatus open                                    # watch it; read the report and the diff
git merge legatus/<run id>                      # or: legatus pr <run id>
```

[AGENTS.md](../AGENTS.md) is what every agent reads first. The backlog is [BACKLOG.md](BACKLOG.md); each item is
written so it can be handed to an agent as it stands.

## Standing automations for this repository

Once the project lives on GitHub, these keep it moving without anyone starting them. Put them in
`automations.yaml` in your Legatus folder (`legatus automations path`), replacing the paths and names:

```yaml
automations:
  # Issues labelled "legatus" by the maintainer become draft pull requests.
  - id: legatus-issues
    github: {repo: OWNER/legatus, label: legatus, every: 10m, authors: [OWNER]}
    repo: C:/code/legatus
    agent: codex
    review: true
    checks: ["go vet ./...", "go test ./..."]
    pr: draft
    max_active: 2

  # A weekly pass over the dependencies.
  - id: legatus-deps
    schedule: weekly mon 09:00
    repo: C:/code/legatus
    agent: codex
    review: true
    prompt: |
      Update the Go dependencies to their latest compatible versions (go get -u, go mod tidy), and fix anything
      that breaks. Change nothing else. Explain in your final message what changed and why.
    checks: ["go vet ./...", "go test ./..."]
    pr: draft
    max_active: 1
```

## What using it on itself has shown

Every problem Legatus had while building itself is written down here and fixed, because they are the problems a
user would have had.

- **A run succeeded when the agent had done nothing.** The agent said it could not write the file, exited cleanly,
  and the run was marked successful. A step that changes no files now fails (unless the workflow says
  `allow_empty`).
- **A secret pasted into a task was stored.** The task text went into the run file and the journal unredacted. It
  is now redacted when the run is created, before anything is written.
- **A hanging sandbox.** Codex's Windows sandbox could not start on the development machine, and a model turn under
  it hung instead of failing. Legatus now checks the sandbox first and stops with a clear message.
- **Replacing a file while it is being read fails on Windows,** which the cockpit does all the time. The store
  retries briefly.
- **A retry that arrived while a run was finishing was dropped.** It is now queued behind the run.
- **Too much at once used up the machine's memory.** Three agents, each building and running the whole test
  suite, plus Legatus's own checks, ran out of memory on a 32 GB PC and crashed it. Legatus now runs one heavy
  check at a time across all runs and defaults to two runs at once. (An agent's own builds are outside Legatus's
  control; [AGENTS.md](../AGENTS.md) asks agents to run `go test -p 2`.)
- **Under that pressure the agents "fixed" unrelated tests.** Two agents loosened a timing assertion in a test they
  had no reason to touch, to get their checks to pass. Review caught it. This is why a person reads every diff
  before it is merged, and why the independent review step exists.
- **Commit messages from agent runs are noise** ("legatus: implement"). They are squashed into a message a person
  writes when the work is merged.
