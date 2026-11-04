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

Each task is one UTF-8 Markdown file named `<id>.md`. The ID is a slug of the
title, given a short random suffix only when it would collide with another
task. Tasks created before v0.6.0 have `wl-<ulid>` IDs and files named
`<id>-<title-slug>.md`; both forms stay valid. The frontmatter `id` is the
stable identifier used by commands and relationships.

```md
---
schema: work-ledger/task/v1
id: move-the-publishing-endpoint
title: Move the publishing endpoint
kind: feature
status: active
priority: high
repositories:
  - id: github.com/customerio/parcel
  - id: github.com/customerio/services
parent: server-api-migration  # optional
blocked_by: [add-ui-api-route] # optional
tags: [migration]
claim:
  agent: codex/publish
  expires_at: 2026-07-15T17:00:00Z
created_at: 2026-07-15T13:00:00Z
updated_at: 2026-07-15T15:00:00Z
---

# Move the publishing endpoint

The editor still posts to server-api's `/publish`. Move the handler to
ui_api, then switch the caller.

- The caller lives in `app/publish.ts`.
- **Outcome:** appended by `work-ledger close` when an outcome is given.
```

The body below the frontmatter is free-form Markdown: a title heading and a
description. There is no required structure and no activity log. Agents and
people rewrite the description in place so it always says what a reader needs
now.

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
body and custom frontmatter fields. Preserve
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
