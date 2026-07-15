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
