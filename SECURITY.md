# Security policy

Legatus starts coding agents on your computer, reads and writes your repositories, and can push branches and open
pull requests when you ask it to. Security reports are taken seriously.

## Reporting a vulnerability

Please **do not open a public issue** for a security problem. Use GitHub's private reporting instead:

**Security tab → "Report a vulnerability"** on this repository.

Include what you found, how to reproduce it, and what you think the impact is. You will get a reply as soon as
the maintainer can manage; this is a small project, so please be patient.

## What is in scope

- Secrets reaching disk, logs, prompts or pull requests (the redaction and the scrubbed environment).
- The daemon accepting a request it should not: wrong host, wrong origin, missing or wrong key, or listening
  beyond the local computer.
- The cockpit executing text that came from an agent or an issue (it builds the page with DOM calls and has a
  strict content policy).
- A workflow, issue, or file in a repository steering an agent past the rules Legatus sets (for example the
  authors list of a GitHub watch).
- Anything that publishes (pushes, opens a pull request) without having been asked.

## What to know

- Legatus is **not an operating-system sandbox**. Running an agent with `--no-sandbox` gives it the access of
  your user account. See [docs/STATUS.md](docs/STATUS.md#sandboxing).
- The daemon's key (the `token` file in the Legatus folder) lets its holder run agents in any repository on the
  machine. Keep it private.

## Supported versions

Only the latest release is supported while the project is at v0.x.
