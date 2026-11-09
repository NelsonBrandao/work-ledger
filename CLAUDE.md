# Work Ledger contributor guide

## Project purpose

Work Ledger is a private, local-first task tracker for people and coding
agents. It stores editable Markdown task files in one central directory,
`~/.work-ledger` by default, rather than in the source repositories a task may
affect. A task can reference more than one repository.

It is for work that does not belong in a team's issue tracker: breakdowns of
large work into child tasks that separate agent sessions pick up, and finds
noticed in passing that are worth doing later. Agents create finds on their
own, so the CLI is built for low ceremony.

This repository contains the Go CLI. Personal agent instructions and private
ledger data do not belong here; see `docs/agent-setup.md` for setting those up
outside a checkout.

## Repository layout

- `cmd/work-ledger/`: the CLI entry point.
- `internal/app/`: argument parsing, command handlers, help/prime output, and
  terminal Markdown rendering.
- `internal/ledger/`: ledger paths, configuration, initialization, and
  permissions.
- `internal/task/`: task schema, Markdown/YAML parsing and rendering, task
  validation, filtering, slug IDs, prefix lookup, and stale-claim checks.
- `internal/store/`: guarded mutations, archive/delete operations, and task
  reference checks.
- `internal/repository/`: Git-origin discovery and normalized repository IDs.
- `internal/lock/`: short-lived advisory file locks for mutations.
- `docs/`: CLI reference, ledger format, setup, and release process.

## Development

Requires Go 1.22.3 or later. The complete verification suite is:

```sh
go test ./...
go vet ./...
go test -race ./...
go build -o /private/tmp/work-ledger ./cmd/work-ledger
```

For focused work, run the relevant package tests, for example:

```sh
go test ./internal/app
go test ./internal/task
```

Run `gofmt -w` on every modified Go file. There is no separate linter
configuration; `go vet` is the static check.

## Design and compatibility rules

- Keep all durable ledger data under the selected ledger home. The fallback is
  `~/.work-ledger`; `WORK_LEDGER_HOME` and the global `--home` flag override
  it. Never write task data into the source repository or initialize Git for a
  ledger.
- Task files are user-editable Markdown with YAML frontmatter. Preserve unknown
  frontmatter when mutating a task, write files atomically, and keep the
  owner-only file and directory permissions enforced by the ledger package.
- A task body is a title heading plus a free-form description. Do not add
  required sections or append log lines from commands; agents rewrite the
  description in place (`edit`), and `close` appends only an optional outcome.
- New IDs are a slug of the title, with a random suffix only on collision
  against active and archived tasks. Commands resolve exact IDs, then legacy
  IDs, then unique prefixes; destructive commands (`delete`) require the full
  ID. Older `wl-<ulid>` IDs and `<id>-<slug>.md` filenames must keep working.
- Treat `--json` output, task frontmatter, task IDs, and normalized repository
  IDs as durable interfaces. Add validation and tests for any format change.
- `work-ledger prime` is the agent's whole setup: personal instructions only
  point at it, so changing its guide changes every agent's behavior on
  upgrade. Keep it short (about 2KB) and neutral about which issue tracker
  the user has; put flag detail in `help <command>`.
- Mutations must respect the existing ETag and advisory-lock behavior. Do not
  silently overwrite a task that changed after it was read.
- Repository-aware operations use normalized Git `origin` remotes and aliases
  from `ledger.yaml`; do not assume a checkout path is a stable identifier.
- When adding or changing a command, update the command handler, `--help`,
  `work-ledger prime`, JSON behavior, tests, and `docs/cli-reference.md`.

## Documentation and releases

- Keep the README concise; the CLI reference and ledger-format documents are
  the detailed user documentation.
- Release source-only semantic-version tags from a clean, up-to-date `main`.
  Follow `docs/releasing.md`: run the full checks, update versioned install
  examples, push `main`, create and push an annotated tag, then publish and
  verify the GitHub release.
- Do not include private ledger task files or claims in a commit. They live outside this repository by design.
