# work-ledger

`work-ledger` is a private, local-first work tracker for people and coding
agents. It stores one editable Markdown file per task in a central folder on
your machine—not in the repositories the task affects and not in a hosted
service.

A task can belong to several repositories at once. That makes it useful for
work that crosses services, applications, and worktrees, while keeping private
breakdowns and findings out of Git and out of the team's issue tracker.

## What it provides

- Human-editable Markdown tasks with structured YAML frontmatter.
- One private ledger shared by all local repositories and worktrees.
- Repository-aware task lookup through normalized Git `origin` remotes.
- Multi-repository tasks, tags, parent/child tasks, and blockers.
- Readable IDs derived from the title, such as `move-api-endpoint`, usable by any unique prefix.
- Agent claims that expire, short-lived mutation locks, and ETag-based conflict checks.
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
go install github.com/NelsonBrandao/work-ledger/cmd/work-ledger@v0.5.0
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

Initialize the private ledger once. This command creates `~/.work-ledger` by
default and never changes the repository you run it from.

Existing ledgers are not moved automatically. Continue using one at the former
default location with `WORK_LEDGER_HOME=~/work-ledger` or
`--home ~/work-ledger`.

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
# Create a task. Its ID is the title as a slug: investigate-api-migration.
work-ledger new "Investigate API migration" --repo . --repo ../services \
  --description "Callers still hit server-api for snippets."

# Any unique prefix of an ID works.
work-ledger show investigate-api

# Claim it for two hours (the default lease).
work-ledger claim investigate-api

# Keep the description current: rewrite it from stdin, or append a paragraph.
work-ledger edit investigate-api < notes.md
work-ledger edit investigate-api --append "Confirmed the caller boundary."

# Split it into pieces other sessions can pick up.
work-ledger split investigate-api "Port the snippets endpoint"

# Finish, or release the claim for someone else.
work-ledger close investigate-api "Moved the endpoint and verified both callers"
work-ledger archive --yes
```

If an agent reads a task that another process may change, it can pass the
`etag` returned by `show --json` as `--if-match` to its later mutation. The
command then fails rather than overwriting a newer edit.

When `show` writes to an interactive terminal, it renders the task body as
Markdown, including syntax-highlighted fenced code blocks. The default
`--color auto` keeps the original Markdown when its output is piped; use
`--color always` to force ANSI output or `--color never` for a rendered
no-color view. `NO_COLOR` disables auto-mode color.

## Documentation

- [CLI reference](docs/cli-reference.md) — every command, flag, and lifecycle operation.
- [Ledger format](docs/ledger-format.md) — directory layout, task schema, manual editing, aliases, and concurrency.
- [Agent setup](docs/agent-setup.md) — using Work Ledger from personal Codex or Claude Code instructions.
- [Release process](docs/releasing.md) — maintainer checklist for future tagged releases.
- [Implementation plan](docs/implementation-plan.md) — original product and technical design record.

## Status and scope

Work Ledger provides the Markdown ledger, task lifecycle, multi-repository
discovery, safe local mutation, agent-oriented JSON output, Beans import, and
rich terminal rendering for task bodies. Prebuilt release binaries,
cross-machine sync, and a TUI are intentionally out of scope.
