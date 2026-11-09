# CLI reference

All commands use this form:

```text
work-ledger [--home PATH] [--json] <command> [arguments]
```

`--home` selects a private ledger directory for this invocation. If it is not
given, Work Ledger uses `WORK_LEDGER_HOME`, then `~/.work-ledger`. `--json`
prints structured command results; errors are written to standard error and use
a non-zero exit status.

Existing ledgers are not moved automatically. To keep using one at the former
default location, set `WORK_LEDGER_HOME=~/work-ledger` or pass
`--home ~/work-ledger`.

Use `work-ledger help <command>` or `work-ledger <command> --help` for a
complete description of that command's options and behavior. Both forms work
without an initialized ledger; `--json` returns the command, usage, purpose,
and detailed notes as structured data.

Every command that takes a task ID accepts the full ID, a legacy ID, or any
unique prefix of the ID; an ambiguous prefix is rejected with the candidates.

Commands that change a task accept `--agent NAME`. When it is omitted, Work
Ledger uses `WORK_LEDGER_AGENT`, then the Claude Code (`CLAUDE_CODE_SESSION_ID`)
or Codex (`CODEX_SESSION_ID`) session, then `manual`. Mutation commands also
accept `--if-match ETAG`, which rejects the change if the task has changed
since a prior `show --json` call.

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
- `prime` prints the short guide an agent reads at the start of a session:
  what belongs in the ledger, when to create a task, how to work one, the
  common commands, the ledger path, and the current repository identity when
  it can be discovered. Personal agent instructions only need to point at it.

## Creating and finding tasks

```sh
work-ledger new "Title" [--description TEXT|-] \
  [--kind task|bug|investigation|feature] \
  [--priority critical|high|normal|low] \
  [--repo PATH-OR-ID]... [--tag TAG]... \
  [--parent TASK-ID] [--blocked-by TASK-ID]...

work-ledger show TASK-ID [--color auto|always|never]
work-ledger list [--all | --here | --repo PATH-OR-ID] \
  [--ready] [--status STATUS]... [--tag TAG]... [--parent TASK-ID]
work-ledger search TEXT...
```

`new` creates a `todo` task and prints its ID. The ID is the title as a slug of
up to six lowercase ASCII words, such as `fix-flaky-login-test`; when that ID is
already taken, active or archived, a random four-character suffix is added
(`fix-flaky-login-test-k3f9`). `--description` sets the task's Markdown
description, and `--description -` reads it from standard input. `create` and
`add` are aliases of `new`, `--title` is accepted in place of the positional
title, and `medium` is accepted as `normal`. Each `--repo`
can be either a Git checkout path or an already-normalized repository ID.
Repeated flags add multiple repositories, tags, and blockers. A task may have
no repositories when it is general planning work.

`show` renders a metadata header (including repositories, tags, relationships,
claim, and timestamps) before the full Markdown body. On an interactive
terminal, Markdown is rendered with headings, lists, links, tables, and
syntax-highlighted fenced code blocks. The default `--color auto` preserves
the raw Markdown body when output is piped; `--color always` forces ANSI output
and `--color never` renders without ANSI. `NO_COLOR` disables color in auto
mode. JSON output includes the original Markdown body and the current `etag`.
`list` searches active tasks only; `--all` is the explicit form for an
unscoped list, `--here` detects the current Git repository, and `--ready`
returns unblocked `todo` work plus stale tasks: tasks still `active` whose
claim expired because the session that held it stopped. JSON summaries include
`stale`, `claim`, `parent`, and the task file's `path`. Multiple `--status` or `--tag` values are
an OR filter within that field. `search` matches titles, structured metadata,
and descriptions case-insensitively. When the words never appear together, it
returns the tasks that match any of them, those matching the most words first.

## Claims, descriptions, and updates

```sh
work-ledger claim TASK-ID [--agent NAME] [--lease DURATION] [--force] [--if-match ETAG]
work-ledger release TASK-ID [--agent NAME] [--force] [--if-match ETAG]
work-ledger edit TASK-ID [--append] [TEXT...] [--agent NAME] [--if-match ETAG]
work-ledger update TASK-ID [--agent NAME] [--if-match ETAG] \
  [--title TITLE] [--kind KIND] [--priority PRIORITY] [--status STATUS] \
  [--parent TASK-ID | --clear-parent] \
  [--repo PATH-OR-ID]... [--clear-repos] \
  [--tag TAG]... [--clear-tags] \
  [--blocked-by TASK-ID]... [--clear-blocked-by]
```

`claim` makes the task `active` and records a two-hour lease by default. Use a
Go duration such as `30m` or `4h` to change it. An unexpired claim held by
another agent is rejected unless `--force` is explicit; an expired one can be
taken over. `release` removes the claim and returns an active task to `todo`;
it releases another agent's unexpired claim only with `--force`.

A task's description is free-form Markdown below its title. `edit` replaces the
whole description with `TEXT`, keeping the title as its heading unless the text
starts with its own `# ` heading; `--append` adds `TEXT` as a new paragraph at
the end instead. Omit `TEXT` to read it from standard input. Agents are
expected to keep the description current, rewriting it as understanding
changes. `note` is an alias of `edit --append` kept for older instructions.

`update` changes only the metadata flags supplied. Supplying any `--repo`,
`--tag`, or `--blocked-by` flags replaces that whole list, so include the values
you want to keep. `--clear-*` explicitly empties the corresponding field.
Allowed statuses are `todo`, `active`, `blocked`, `done`, and `cancelled`;
common spellings such as `in-progress` and `closed` are accepted for them.

## Relationships and lifecycle

```sh
work-ledger split PARENT-ID "Child title" [new-task options]
work-ledger close TASK-ID ["Outcome"] [--agent NAME] [--force] [--if-match ETAG]
work-ledger archive --yes
work-ledger delete TASK-ID --yes
```

`split` creates a child task. It inherits the parent's repositories and tags
unless you provide replacements. `close` marks the task `done`, clears its
claim, and appends the optional outcome to the description as
`**Outcome:** ...`. It refuses to close a task another agent holds an
unexpired claim on unless `--force` is explicit.

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
