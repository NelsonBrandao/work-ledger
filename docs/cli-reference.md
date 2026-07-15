# CLI reference

All commands use this form:

```text
work-ledger [--home PATH] [--json] <command> [arguments]
```

`--home` selects a private ledger directory for this invocation. If it is not
given, Work Ledger uses `WORK_LEDGER_HOME`, then `~/work-ledger`. `--json`
prints structured command results; errors are written to standard error and use
a non-zero exit status.

Commands that change a task accept `--agent NAME`. When it is omitted, Work
Ledger uses `WORK_LEDGER_AGENT`, then `manual`. Mutation commands also accept
`--if-match ETAG`, which rejects the change if the task has changed since a
prior `show --json` call.

## Setup and diagnosis

```sh
work-ledger init
work-ledger doctor
work-ledger context
work-ledger prime
```

- `init` creates the ledger directory, configuration, task directories, and
  runtime-lock directory. It is safe to run again and never initializes Git.
- `doctor` validates the configuration, active tasks, archived tasks, IDs, and
  parent/blocker references.
- `context` shows the current Git checkout's normalized `origin` identifier
  and the ledger location. It can run before initialization and reports that
  state in its output.
- `prime` prints a self-contained agent guide: the required operating protocol,
  every command's syntax and meaningful options, valid metadata values, the
  private ledger path, and the current repository identity when it can be
  discovered.

## Creating and finding tasks

```sh
work-ledger new "Title" \
  [--kind task|bug|investigation|feature] \
  [--priority critical|high|normal|low] \
  [--repo PATH-OR-ID]... [--tag TAG]... \
  [--parent TASK-ID] [--blocked-by TASK-ID]...

work-ledger show TASK-ID
work-ledger list [--all | --here | --repo PATH-OR-ID] \
  [--ready] [--status STATUS]... [--tag TAG]... [--parent TASK-ID]
work-ledger search TEXT...
```

`new` creates a `todo` task and prints its generated `wl-...` ID. Each `--repo`
can be either a Git checkout path or an already-normalized repository ID.
Repeated flags add multiple repositories, tags, and blockers. A task may have
no repositories when it is general planning work.

`show` includes the full Markdown body and, in JSON output, the current `etag`.
`list` searches active tasks only; `--all` is the explicit form for an
unscoped list, `--here` detects the current Git repository, and `--ready`
returns only unblocked `todo` work. Multiple `--status` or `--tag` values are
an OR filter within that field. `search` matches titles, structured metadata,
and Markdown bodies case-insensitively.

## Claims, notes, and updates

```sh
work-ledger claim TASK-ID [--agent NAME] [--lease DURATION] [--force] [--if-match ETAG]
work-ledger release TASK-ID [--agent NAME] [--force] [--if-match ETAG]
work-ledger note TASK-ID [TEXT...] [--agent NAME] [--if-match ETAG]
work-ledger update TASK-ID [--agent NAME] [--if-match ETAG] \
  [--title TITLE] [--kind KIND] [--priority PRIORITY] [--status STATUS] \
  [--parent TASK-ID | --clear-parent] \
  [--repo PATH-OR-ID]... [--clear-repos] \
  [--tag TAG]... [--clear-tags] \
  [--blocked-by TASK-ID]... [--clear-blocked-by]
```

`claim` makes the task `active` and records a two-hour lease by default. Use a
Go duration such as `30m` or `4h` to change it. An unexpired claim held by
another agent is rejected unless `--force` is explicit. `release` removes the
claim and returns an active task to `todo`; it only releases another agent's
claim with `--force`.

`note` appends a timestamped entry to the task's `## Activity` section. Omit
`TEXT` to read the note from standard input.

`update` changes only the metadata flags supplied. Supplying any `--repo`,
`--tag`, or `--blocked-by` flags replaces that whole list, so include the values
you want to keep. `--clear-*` explicitly empties the corresponding field.
Allowed statuses are `todo`, `active`, `blocked`, `done`, and `cancelled`.

## Relationships and lifecycle

```sh
work-ledger split PARENT-ID "Child title" [new-task options]
work-ledger close TASK-ID "Closing summary" [--agent NAME] [--force] [--if-match ETAG]
work-ledger archive --yes
work-ledger delete TASK-ID --yes
```

`split` creates a child task. It inherits the parent's repositories and tags
unless you provide replacements. `close` marks the task `done`, clears its
claim, and records the required closing summary in Activity. It refuses to
close a task claimed by another agent unless `--force` is explicit.

`archive --yes` moves all `done` and `cancelled` tasks to `archive/<year>/`.
`delete --yes` permanently deletes an active task only when no task, including
an archived one, names it as a parent or blocker.

## Importing Beans

```sh
work-ledger import beans PATH [--dry-run] [--repo PATH-OR-ID]...
```

The importer reads a Beans directory and never modifies it. Use `--dry-run`
first to validate the source and see the task count. A real import creates new
Work Ledger IDs, records each original Beans ID under `legacy_ids`, retains the
Markdown body, and remaps parent/blocker relationships within that import.
Imported source status is mapped to `done`, `cancelled`, or `todo`; claims are
not imported. Re-importing a Beans ID already recorded in the ledger is
rejected.
