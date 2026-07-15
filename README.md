# work-ledger

`work-ledger` is a private, local-first work tracker for people and coding
agents. It stores one editable Markdown file per task in a central folder on
your machine—not in the repositories the task affects and not in a hosted
service.

A task can belong to several repositories at once. That makes it useful for
work that crosses services, applications, and worktrees, while keeping plans,
investigations, and agent activity out of Git.

## What it provides

- Human-editable Markdown tasks with structured YAML frontmatter.
- One private ledger shared by all local repositories and worktrees.
- Repository-aware task lookup through normalized Git `origin` remotes.
- Multi-repository tasks, tags, parent/child tasks, and blockers.
- Agent claims, short-lived mutation locks, and ETag-based conflict checks.
- JSON output for agent automation, plus a migration path from Beans.

Work Ledger is deliberately single-machine in `v0.1.0`. It does not sync data,
create a Git repository for the ledger, or send ledger contents over the
network. Back up the ledger using your normal private backup solution.

## Requirements

- Go 1.22.3 or later to install from source.
- Git when using repository-aware commands such as `context` and `list --here`.
- Access to this private GitHub repository when installing with `go install`.

## Install

Install the released CLI without cloning a working checkout:

```sh
go env -w GOPRIVATE=github.com/NelsonBrandao/*
go install github.com/NelsonBrandao/work-ledger/cmd/work-ledger@v0.1.0
```

`GOPRIVATE` tells Go to fetch this private module directly rather than through
the public module proxy and checksum database. GitHub authentication must
already work for your account (through SSH, a credential manager, or a token).
The resulting binary is installed to `$GOBIN`, or `$(go env GOPATH)/bin` when
`GOBIN` is unset; ensure that directory is on your `PATH`.

For development from a checkout:

```sh
go build -o work-ledger ./cmd/work-ledger
./work-ledger help
```

## Quick start

Initialize the private ledger once. This command creates `~/work-ledger` by
default and never changes the repository you run it from.

```sh
work-ledger init
work-ledger doctor
```

Create one task that touches multiple repositories, then find it from either
checkout:

```sh
cd ~/code/parcel
work-ledger new "Move the publishing endpoint" --repo . --repo ../services --tag migration
work-ledger list --here --ready
work-ledger context
```

Use another private location by setting `WORK_LEDGER_HOME` or passing the
global `--home` option:

```sh
export WORK_LEDGER_HOME="$HOME/Library/Application Support/work-ledger"
work-ledger init
work-ledger --home /private/path/to/ledger list --all
```

## A typical task workflow

```sh
# Create a task and copy the returned ID.
work-ledger new "Investigate API migration" --repo . --repo ../services

# Inspect it. JSON output includes `etag` for guarded updates.
work-ledger --json show wl-01...

# An agent claims the task for two hours by default.
work-ledger claim wl-01... --agent codex/api-migration
work-ledger note wl-01... "Confirmed the caller boundary" --agent codex/api-migration

# Protect a read-modify-write operation with the ETag from `show --json`.
work-ledger update wl-01... --priority high --if-match <etag> --agent codex/api-migration

# Finish or release the claim for someone else.
work-ledger close wl-01... "Moved the endpoint and verified both callers" --agent codex/api-migration
work-ledger archive --yes
```

If an agent reads a task that another process may change, it should pass the
`etag` returned by `show --json` as `--if-match` to its later mutation. The
command fails rather than overwriting a newer edit.

## Documentation

- [CLI reference](docs/cli-reference.md) — every command, flag, and lifecycle operation.
- [Ledger format](docs/ledger-format.md) — directory layout, task schema, manual editing, aliases, and concurrency.
- [Agent setup](docs/agent-setup.md) — using Work Ledger from personal Codex or Claude Code instructions.
- [Release process](docs/releasing.md) — maintainer checklist for future tagged releases.
- [Implementation plan](docs/implementation-plan.md) — original product and technical design record.

## Status and scope

`v0.1.0` is the first usable release. It provides the Markdown ledger, task
lifecycle, multi-repository discovery, safe local mutation, agent-oriented
JSON output, and Beans import. Prebuilt release binaries, cross-machine sync,
and a TUI are intentionally out of scope for this release.
