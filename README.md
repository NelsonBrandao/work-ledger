# work-ledger

A private, local-first work tracker for people and coding agents.

Work Ledger keeps one manually editable Markdown file per task in a central
folder outside source repositories. A task can reference multiple repositories,
so the same work is visible from every associated checkout and worktree without
placing task data in Git.

## Intended first release

- Central Markdown workspace, configured by the user.
- YAML frontmatter for task state, ownership, dependencies, and repository
  associations.
- Markdown sections for plans, investigations, and activity.
- A small CLI that discovers the current repository from its Git remote and
  lists the relevant tasks.
- Safe concurrent updates using per-task locks and atomic file replacement.

The project is intentionally local-only for its first release. Live sync across
machines, a TUI, and a hosted service are not initial goals.

## Quick start

```sh
go build -o work-ledger ./cmd/work-ledger
./work-ledger init
./work-ledger new "Investigate API migration" --repo ../parcel --repo ../services
./work-ledger list --here --ready
```

Tasks are written under `~/work-ledger` by default. Set `WORK_LEDGER_HOME` or
pass `--home` to use a different private directory.

## Commands

The CLI supports task creation, show/list/search, claims, notes, metadata
updates with ETag checks, task splitting, close/archive/delete lifecycle
operations, agent priming, and Beans imports. Run `work-ledger help` for the
command list and [the implementation plan](docs/implementation-plan.md) for the
file format and concurrency rules. See [agent setup](docs/agent-setup.md) for
personal Codex or Claude Code guidance.
