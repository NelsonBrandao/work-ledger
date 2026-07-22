# Work Ledger implementation plan

## 1. Product contract

Work Ledger is a private, local-first command-line tracker for people and coding
agents. Its authoritative data is a user-owned folder of Markdown files, not a
Git repository and not a database. One work item may concern several source
repositories.

The first release must satisfy these outcomes:

- A person can read and edit every task directly in an editor.
- Task data never appears in a source repository, its `.git` directory, or a
  remote service.
- The same task is discoverable from every associated repository and worktree.
- Several local agents can safely work on different tasks, and only one can
  claim a task at a time.
- Agents can record a plan, investigation notes, dependencies, and a closing
  summary without a separate planning tool.

The first release is intentionally single-machine. It does not include a TUI,
web UI, hosted service, Git-backed sync, encryption beyond normal filesystem
permissions, or a derived search index.

## 2. Technical decisions

Implement a standalone Go CLI named `work-ledger`. A compiled binary avoids requiring
Node, Python, or a particular agent runtime in every checkout. Keep dependencies
small: standard-library filesystem, process, JSON, and crypto packages plus a
YAML parser. Choose the exact Go version when creating `go.mod`.

The CLI scans task files on demand. This is simple, makes manual edits visible
immediately, and is fast for the expected hundreds or low thousands of tasks.
An index can be added later only after profiling proves it necessary.

The default ledger directory is `~/.work-ledger`; `WORK_LEDGER_HOME` and a global
`--home` flag override it. All durable configuration, tasks, archives, and
runtime lock files live below this directory. `work-ledger init` never changes the
current source repository.

```text
~/.work-ledger/
  ledger.yaml                 # format version and repository aliases
  tasks/                      # active task Markdown files
  archive/2026/               # closed or cancelled task Markdown files
  .runtime/locks/             # disposable advisory locks
  templates/task.md           # optional user-editable creation template
```

New directories use mode `0700`; files use mode `0600`. The tool sends no data
over the network. The user remains responsible for full-disk encryption and
backups.

## 3. File format

Each work item is one UTF-8 Markdown file. Its filename is an implementation
detail (`wl-<ULID>-<slug>.md`); its frontmatter `id` is the stable reference.
YAML frontmatter carries queryable state, while the Markdown body remains free
form and is the place for the plan and investigation.

```md
---
schema: work-ledger/task/v1
id: wl-01J...
title: Migrate publish API
kind: task                    # task | bug | investigation | feature
status: active                # todo | active | blocked | done | cancelled
priority: normal              # critical | high | normal | low
repositories:
  - id: github.com/customerio/parcel
    paths: [app/src/features/publish]
  - id: github.com/customerio/services
    paths: [ui_api/publish]
parent: wl-01H...             # optional
blocked_by: [wl-01G...]       # optional
tags: [migration]
claim:
  agent: codex/root
  expires_at: 2026-07-15T15:00:00Z
created_at: 2026-07-15T13:00:00Z
updated_at: 2026-07-15T13:30:00Z
legacy_ids: [parcel-vazf]     # optional, used by the Beans importer
---

# Migrate publish API

## Plan

- [ ] Move the endpoint.

## Investigation

Record decisions, links, and command output here.

## Activity

- 2026-07-15T13:30:00Z — codex/root: Claimed the task.
```

`title` is authoritative; the H1 is a human-readable mirror. `work-ledger doctor`
reports a mismatch but never changes a task unless explicitly asked. The CLI
owns the known frontmatter fields and the generated Activity entries. It must
preserve every other frontmatter key and all body text. A malformed or manually
edited task is reported as invalid and is never silently repaired.

Statuses and dependency rules:

- `todo` is available unless a `blocked_by` task is unfinished.
- `active` has an unexpired claim. A claim may be released or replaced after
  expiry.
- `blocked` requires a non-empty `blocked_by` list or an explanatory note.
- `done` requires a closing summary in Activity; `cancelled` requires a reason.
- A parent does not automatically inherit the state of children. `work-ledger ready`
  shows only unblocked `todo` work.

## 4. Repository identity and multi-repository work

Repository IDs are stable, normalized remote names such as
`github.com/customerio/parcel`. From a Git checkout, `work-ledger` resolves the
current context without network access:

1. Run `git rev-parse --show-toplevel` and `git remote get-url origin`.
2. Normalize SSH and HTTPS remotes by removing the scheme or `git@` prefix and
   a trailing `.git` suffix.
3. Look up the normalized remote in `ledger.yaml` aliases; otherwise use it as
   the repository ID.
4. Filter task frontmatter for that ID.

`ledger.yaml` supports forks and unusual remotes without putting configuration
in the checkout:

```yaml
schema: work-ledger/config/v1
repository_aliases:
  github.com/customerio/parcel:
    - github.com/NelsonBrandao/parcel
```

Tasks can name any number of repositories. Creation accepts a repository path,
an ID, or the current directory:

```text
work-ledger new "Migrate publish API" --repo . --repo ../services
work-ledger list --here
work-ledger list --repo github.com/customerio/services
```

`--here` works in any worktree because worktrees normally share the same remote.
For a non-Git directory, `--repo <id>` is required. Untagged planning tasks are
visible through `work-ledger list --all`, not through `--here`.

## 5. CLI surface

All commands provide a stable `--json` result for agents and a concise
human-readable default. Errors use non-zero exits and structured JSON errors
when `--json` is selected.

| Command | First-release behavior |
| --- | --- |
| `work-ledger init` | Creates and validates a ledger directory; never initializes Git. |
| `work-ledger context` | Prints detected repository identity and ledger path. |
| `work-ledger new` | Creates a Markdown task, resolving repeated `--repo` values. |
| `work-ledger show <id>` | Renders one task, including claim and dependency status. |
| `work-ledger list` | Filters by state, tag, repository, parent, and `--ready`; supports `--here`. |
| `work-ledger search <text>` | Case-insensitive scan of task title, frontmatter, and Markdown body. |
| `work-ledger claim <id>` | Atomically creates or renews a time-bound claim. |
| `work-ledger release <id>` | Releases the caller's claim. |
| `work-ledger note <id>` | Appends a timestamped Activity entry from an argument or stdin. |
| `work-ledger update <id>` | Changes explicitly named metadata only, with `--if-match` conflict protection. |
| `work-ledger split <id>` | Creates a child task that inherits repositories unless overridden. |
| `work-ledger close <id>` | Marks done and records a required summary. |
| `work-ledger archive` | Moves done/cancelled tasks into an archive year after confirmation. |
| `work-ledger doctor` | Validates frontmatter, references, task filenames, and repository aliases. |
| `work-ledger import beans <path>` | Imports a Beans directory without modifying it. |
| `work-ledger prime` | Prints concise agent instructions and current context. |

The first implementation may defer `search`, `archive`, and `prime` until the
core read/write commands are proven. It must not implement GraphQL, a roadmap,
or arbitrary file-body replacement.

## 6. Concurrency and manual editing

One task per file means unrelated tasks never contend. Mutating commands use a
short-lived advisory lock at `.runtime/locks/<task-id>`:

1. Create the lock atomically and record PID, hostname, and expiry.
2. Read and validate the current task after obtaining the lock.
3. Apply the requested mutation, preserve unmanaged content, and update
   `updated_at`.
4. Write a same-directory temporary file, `fsync`, and atomically rename it
   over the original.
5. Remove the lock.

Locks expire after a short, documented duration so a crashed agent cannot block
work forever. `claim` is a separate, durable lease in task frontmatter; it
prevents two agents from actively owning the same task. The command rejects an
unexpired claim held by another agent unless an explicit override is supplied.

`work-ledger show --json` exposes a content hash. `work-ledger update --if-match <hash>`
rejects a stale caller, which is how agents protect a read-modify-write step.
Direct editor changes are supported: the next command parses the changed file.
The tool cannot prevent an editor from overwriting a simultaneous CLI update,
so it reports conflicts rather than guessing how to merge Markdown.

## 7. Code layout and implementation order

```text
cmd/work-ledger/main.go
internal/app/          # command orchestration and exit/error policy
internal/cli/          # flag parsing and human/JSON rendering
internal/ledger/       # home discovery, init, paths, and configuration
internal/task/         # task schema, Markdown/frontmatter parsing, validation
internal/store/        # scanning, atomic write, archive, and task lookup
internal/lock/         # advisory lock lifecycle
internal/repository/   # Git remote discovery and alias resolution
internal/importer/     # Beans importer
testdata/              # valid, invalid, legacy, and concurrent task fixtures
```

### Phase 0 — bootstrap and contracts

- Create the Go module, binary entry point, formatter/linter configuration, and
  test harness.
- Add fixtures for valid and invalid task Markdown.
- Implement the task/config parser and `work-ledger init`, `work-ledger doctor`, and
  `work-ledger context`.
- Acceptance: a new ledger is created outside a Git checkout; malformed files
  receive clear file-and-field errors.

### Phase 1 — read path and task creation

- Implement ID/slug generation, template rendering, task scanning, and
  repository normalization.
- Implement `new`, `show`, `list`, `list --here`, `list --ready`, and `search`.
- Acceptance: a multi-repository task created from Parcel appears from Services
  and all worktrees of either repository, with no changes in either checkout.

### Phase 2 — safe mutation and lifecycle

- Implement advisory locks, atomic writes, content hashes, claims, notes,
  metadata updates, dependencies, `split`, `release`, and `close`.
- Acceptance: two concurrent `claim` processes produce exactly one winner;
  expired claims become claimable; no partial Markdown file is observable.

### Phase 3 — agent ergonomics

- Implement `prime` and JSON output contracts.
- Write global Codex and Claude Code installation guidance that points to the
  binary; do not alter per-repository `AGENTS.md` by default.
- Acceptance: an agent can identify/claim a task, add a note, and close it using
  only JSON commands and a supplied task ID.

### Phase 4 — migration and operating tools

- Implement a dry-run Beans importer, status/type mapping, relationship remap,
  and `legacy_ids` preservation.
- Implement archive and backup/export guidance.
- Acceptance: importing a fixture preserves task count, bodies, checklists,
  parent/blocker relationships, and does not write to the input `.beans`
  directory or any source repository.

### Phase 5 — release hardening

- Build macOS Apple Silicon and Linux binaries, publish checksums, and document
  installation/upgrade/rollback.
- Test permission failures, interrupted writes, symlink handling, aliases,
  spaces in paths, and a ledger with at least 1,000 tasks.
- Acceptance: `work-ledger doctor`, `work-ledger list --here`, and the mutation commands have
  deterministic exit codes and documented recovery steps.

## 8. Test strategy

Use table-driven unit tests for YAML frontmatter, Markdown preservation, ID
creation, state transitions, Git-remote normalization, aliases, and dependency
resolution. Use temporary directories for command-level tests; create bare Git
repositories and worktrees locally rather than calling the network.

Add process-level concurrency tests for locks and claims. Test crash recovery by
creating an expired lock/lease fixture. Test manual-edit compatibility by adding
unknown frontmatter and custom Markdown sections, running a mutation, and
asserting that only CLI-owned metadata and the Activity insertion changed.

Every command that is executed from a Git fixture must assert `git status
--porcelain` is unchanged afterward. This is the core privacy regression test.

## 9. Migration and rollout

1. Install an unreleased binary and initialize a separate test ledger.
2. Import a copy of the current `.beans` directory with `--dry-run`; compare
   totals, states, parent/blocker links, and representative long investigation
   bodies.
3. Import for real without deleting `.beans`; use Work Ledger for new tasks for
   a trial period.
4. Replace personal/global agent instructions after the trial. Keep existing
   repository instructions only when intentionally sharing a workflow.
5. Retain the Beans directory as a read-only backup until the importer and
   daily use are trusted, then archive it outside source repositories.

No task data is pushed to the `work-ledger` source repository. That repository
contains the program, tests, documentation, and release artifacts only.

## 10. Deferred decisions

- Cross-machine sync needs conflict-aware replication or a small authenticated
  service; do not use Dropbox/iCloud-style sync for active task files.
- Encryption at rest, shared users, permissions/roles, and notifications are
  future features.
- A generated static dashboard can be added later from the same Markdown files;
  it must remain a view, not a second source of truth.
- Evaluate an optional cache only if full scans become measurably slow.
