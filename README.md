# work-ledger

[![CI](https://github.com/NelsonBrandao/work-ledger/actions/workflows/ci.yml/badge.svg)](https://github.com/NelsonBrandao/work-ledger/actions/workflows/ci.yml)

A private backlog for you and your coding agents. Work Ledger keeps one
Markdown file per task in a folder on your machine (`~/.work-ledger`), outside
your repositories and outside any hosted service.

It is built for two kinds of work that do not belong in your team's issue
tracker:

- **Breakdowns.** A large piece of work split into child tasks, so several
  agent sessions can pick them up one at a time or in parallel.
- **Finds.** Something you or an agent noticed in passing and want to come
  back to: an odd log line, a latent bug, a refactor, one piece of a bigger
  problem.

Agents create finds on their own and tell you in one line. You read and edit
the same files in any editor.

## Install

Requires Go 1.22.3 or later.

```sh
go install github.com/NelsonBrandao/work-ledger/cmd/work-ledger@v0.6.0
work-ledger init
```

`go install` puts the binary in `$GOBIN`, or `$(go env GOPATH)/bin` when
`GOBIN` is unset; make sure that directory is on your `PATH`. `init` creates
`~/.work-ledger`. Set `WORK_LEDGER_HOME` to keep the ledger somewhere else.

## Set up your agent

Add one line to your personal agent instructions: `~/.claude/CLAUDE.md` for
Claude Code, `~/.codex/AGENTS.md` for Codex.

```md
## Work Ledger

Run `work-ledger prime` at the start of each session and follow it.
```

`prime` prints a short guide (about 2KB) that ships with the CLI: what belongs
in the ledger, when an agent should create a task, and the commands to use.
Upgrading `work-ledger` updates what every agent reads. See
[Agent setup](docs/agent-setup.md) for details and how to check it worked.

## Use it yourself

```sh
# Create a task. Its ID is the title as a slug: investigate-api-migration.
work-ledger new "Investigate API migration" --repo . --repo ../api \
  --description "Callers still hit the old service for snippets."

# Any unique prefix of an ID works.
work-ledger show investigate-api

# See what is ready to pick up in the current repository.
work-ledger list --here --ready

# Split it into pieces other sessions can take.
work-ledger split investigate-api "Port the snippets endpoint"

# Claim, keep the description current, and close with the outcome.
work-ledger claim investigate-api
work-ledger edit investigate-api --append "Confirmed the caller boundary."
work-ledger close investigate-api "Moved the endpoint and verified both callers"
```

A task file looks like this:

```md
---
schema: work-ledger/task/v1
id: investigate-api-migration
title: Investigate API migration
kind: task
status: todo
priority: normal
repositories:
    - id: github.com/acme/web
    - id: github.com/acme/api
created_at: "2026-11-09T10:00:00Z"
updated_at: "2026-11-09T10:00:00Z"
---

# Investigate API migration

Callers still hit the old service for snippets.
```

## How it works

- **One ledger for every checkout.** A task can name several repositories.
  `list --here` finds tasks for the repository you are in by its normalized
  Git `origin`, so it works the same from any clone or worktree.
- **Readable IDs.** New IDs come from the title. A short random suffix is
  added only when the ID is taken, and every command accepts a unique prefix.
- **Free-form descriptions.** There is no required template and no activity
  log. Agents rewrite the description so it says what the next reader needs.
- **Claims that expire.** `claim` gives a session a two-hour lease by default.
  When a session ends without releasing it, the task shows as `stale` in
  `list --ready` and any agent can take it over.
- **Safe concurrent edits.** Every change takes a short per-task lock and
  replaces the file atomically. `--if-match` with the `etag` from
  `show --json` refuses a change when the task was edited since you read it.
- **Agent-friendly output.** Every command takes `--json`.

Work Ledger runs on one machine. It does not sync, start a server, or send
ledger contents anywhere. Back up `~/.work-ledger` the way you back up other
private files.

## Documentation

- [CLI reference](docs/cli-reference.md): every command and flag.
- [Ledger format](docs/ledger-format.md): task files, manual editing, repository aliases, and concurrency.
- [Agent setup](docs/agent-setup.md): Claude Code and Codex configuration.
- [Releasing](docs/releasing.md): maintainer checklist.

## Development

```sh
go test ./...
go vet ./...
go test -race ./...
go build ./cmd/work-ledger
```

## License

[Apache License 2.0](LICENSE)
