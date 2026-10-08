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

This section is kept honest on purpose: every problem Legatus had while building itself is written down here and
fixed, because those are the problems a user would have had.

(see the entries below, newest last)
