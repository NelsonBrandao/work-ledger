# Ledger format and manual editing

Work Ledger's durable data lives in one private directory. The CLI does not
write task data into the source repository from which it is invoked.

## Directory layout

```text
~/.work-ledger/
  ledger.yaml              # schema version and repository aliases
  tasks/                   # active task Markdown files
  archive/<year>/          # completed or cancelled task files
  .runtime/locks/          # disposable advisory locks
  templates/               # reserved for future task-template support
```

`init` creates directories with owner-only permissions where the operating
system permits it (`0700` for directories and `0600` for files). The ledger is
still ordinary local data: use full-disk encryption and a private backup
strategy appropriate for your machine.

## Task files

Each task is one UTF-8 Markdown file. The filename is derived from its ID and
title, but the frontmatter `id` is the stable identifier used by commands and
relationships.

```md
---
schema: work-ledger/task/v1
id: wl-01j...
title: Move the publishing endpoint
kind: feature
status: active
priority: high
repositories:
  - id: github.com/customerio/parcel
  - id: github.com/customerio/services
parent: wl-01h...             # optional
blocked_by: [wl-01g...]       # optional
tags: [migration]
claim:
  agent: codex/publish
  expires_at: 2026-07-15T17:00:00Z
created_at: 2026-07-15T13:00:00Z
updated_at: 2026-07-15T15:00:00Z
---

# Move the publishing endpoint

## Plan

- [ ] Update the caller.

## Investigation

Record decisions, links, and useful command output here.

## Activity

- 2026-07-15T15:00:00Z — codex/publish: Claimed task until 2026-07-15T17:00:00Z.
```

Valid values are:

| Field | Values |
| --- | --- |
| `kind` | `task`, `bug`, `investigation`, `feature` |
| `status` | `todo`, `active`, `blocked`, `done`, `cancelled` |
| `priority` | `critical`, `high`, `normal`, `low` |

`repositories`, `parent`, `blocked_by`, `tags`, `claim`, and `legacy_ids` are
optional. A repository can also contain a `paths` list for human context;
current filtering is by repository ID, not by individual path.

## Editing safely by hand

Manual editing is a first-class workflow. You can freely edit the Markdown
body, checklists, plan, investigation, and custom frontmatter fields. Preserve
the required frontmatter fields (`schema`, `id`, `title`, `kind`, `status`,
`priority`, `repositories`, `created_at`, and `updated_at`) and keep referenced
parent and blocker IDs valid.

The CLI preserves unknown frontmatter fields when it mutates a task. Run
`work-ledger doctor` after a substantial manual change to validate the ledger.
Do not manually edit the same task while an agent is making a CLI mutation; use
the ETag from `work-ledger --json show <id>` for a guarded read-modify-write
workflow instead.

## Repository identities and aliases

`context` and `list --here` read the current checkout's `origin` remote and
normalize SSH and HTTPS forms to an ID such as
`github.com/customerio/parcel`. This happens locally and does not contact the
remote service.

Use `ledger.yaml` to make a fork or unusual remote resolve to the canonical
repository ID used by tasks:

```yaml
schema: work-ledger/config/v1
repository_aliases:
  github.com/customerio/parcel:
    - github.com/NelsonBrandao/parcel
```

## Concurrency and recovery

Every mutation takes a short-lived, per-task advisory lock and atomically
replaces the task file. Locks expire after one minute if a process crashes.
Claims are separate durable leases that coordinate who is actively working on a
task; they expire after two hours by default.

Use a claim to express ownership and an ETag to protect a specific update. If a
lock or ETag check fails, re-read the task, decide how to merge the change, and
try again. Work Ledger does not attempt to merge simultaneous editor and CLI
edits automatically.

Archived tasks stay readable under `archive/` and are included when validating
relationships. Use your normal filesystem backup tooling to protect the whole
ledger directory.
