# Contributing to Legatus

Thank you for looking. Legatus is early, so bug reports, ideas and small, well-tested changes are all welcome.

## Before you start

- **Open an issue first** for anything bigger than a small fix, so we can agree on the direction before you spend
  time on it. The [backlog](docs/BACKLOG.md) lists what is already wanted and is a good place to start.
- **Security problems** do not go in public issues. See [SECURITY.md](SECURITY.md).

## Building and testing

You need Go 1.26 or newer and `git`. Node.js is optional (one test checks the cockpit's JavaScript syntax when it
is installed).

```text
gofmt -l .            # prints nothing
go vet ./...
go test ./...         # a few minutes: some tests start real git processes
go build -o legatus ./cmd/legatus
```

On a small machine, `go test -p 2 ./...` uses less memory.

## What a good change looks like

- **It has a test.** Every change in behaviour has a test that fails without it. Tests never need the network, a
  signed-in agent or GitHub; real-world checks are gated behind an environment variable. Use the stand-ins that are
  already there (`internal/agent/fake`, the scripted `gh` in `internal/github`).
- **It says what is true.** If you add a claim to the documentation, add the evidence too, or say it is unverified.
  [docs/STATUS.md](docs/STATUS.md) is where verification is recorded.
- **It keeps the safety properties.** Secrets are redacted before storage, agents get a scrubbed environment,
  nothing is published without being asked, the server listens only on the local computer. Weakening any of these
  needs a clear reason and a test.
- **It works on Windows, macOS and Linux,** and adds no dependency without a strong reason (there is one today).
- **It is small and focused.** One change per pull request, with a message that says why.

[AGENTS.md](AGENTS.md) has the same rules in the form a coding agent can follow, and
[docs/DOGFOOD.md](docs/DOGFOOD.md) describes how the project uses itself.

## Pull requests

Describe what changed and how you checked it. Say what you did not check. CI runs on all three systems; a pull
request needs it green.

By contributing you agree that your work is released under the [MIT license](LICENSE).
