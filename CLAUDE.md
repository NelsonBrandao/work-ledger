# Work Ledger contributor guide

## Project purpose

Work Ledger is a private, local-first task tracker for people and coding
agents. It stores editable Markdown task files in one central directory,
`~/.work-ledger` by default, rather than in the source repositories a task may
affect. A task can reference more than one repository.

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
  validation, filtering, and activity history.
- `internal/store/`: guarded mutations, archive/delete operations, and task
  reference checks.
- `internal/repository/`: Git-origin discovery and normalized repository IDs.
- `internal/importer/`: Beans migration.
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
- Treat `--json` output, task frontmatter, task IDs, and normalized repository
  IDs as durable interfaces. Add validation and tests for any format change.
- Mutations must respect the existing ETag and advisory-lock behavior. Do not
  silently overwrite a task that changed after it was read.
- Repository-aware operations use normalized Git `origin` remotes and aliases
  from `ledger.yaml`; do not assume a checkout path is a stable identifier.
- The Beans importer applies repository scopes only from explicit `--repo`
  values. Preserve that documented behavior unless changing the importer and
  its tests intentionally.
- When adding or changing a command, update the command handler, `--help`,
  `work-ledger prime`, JSON behavior, tests, and `docs/cli-reference.md`.

## Documentation and releases

- Keep the README concise; the CLI reference and ledger-format documents are
  the detailed user documentation.
- Release source-only semantic-version tags from a clean, up-to-date `main`.
  Follow `docs/releasing.md`: run the full checks, update versioned install
  examples, push `main`, create and push an annotated tag, then publish and
  verify the GitHub release.
- Do not include private ledger files, claims, plans, or agent notes in a
  commit. They live outside this repository by design.
