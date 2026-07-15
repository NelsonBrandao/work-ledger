# Agent setup

Work Ledger is meant to be installed once on a developer machine and referenced
from personal Codex or Claude Code instructions. Do not add the ledger directory
or task files to a source repository.

## 1. Make the binary available

Install the released binary with Go; a source checkout is not required:

```sh
go env -w GOPRIVATE=github.com/NelsonBrandao/*
go install github.com/NelsonBrandao/work-ledger/cmd/work-ledger@v0.1.0
```

Ensure `$GOBIN` (or `$(go env GOPATH)/bin`) is on `PATH`, then initialize the
private ledger once:

```sh
work-ledger init
```

Use `WORK_LEDGER_HOME` or `--home` when the ledger should live somewhere other
than `~/work-ledger`. Do not put this directory, its configuration, or task
files in a source repository.

## 2. Add this to personal agent instructions

Adapt the following to your global Codex instructions, Claude Code user
instructions, or a personal agent skill. It is deliberately not a repository
`AGENTS.md` rule.

```text
For non-trivial work, run `work-ledger prime` and follow its guidance.
Use `work-ledger --json list --here --ready` to find work relevant to the
current checkout. Claim or create a task before making changes. Keep notable
decisions in `work-ledger note`, and close completed work with a summary.
When handing work to another agent, supply the work-ledger task ID.
When updating a task after reading it, use the `etag` from
`work-ledger --json show <id>` as `--if-match`.
```

## 3. Multi-repository work

Create one task with every repository it affects:

```sh
work-ledger new "Move API endpoint" --repo ../parcel --repo ../services
```

The tool identifies a checkout from its normalized `origin` remote. Use
`work-ledger context` to inspect that identity, and define fork aliases in the
private `ledger.yaml` file when necessary.

## 4. Concurrent agents

`claim` creates a time-limited claim, while every mutation takes a short-lived
per-task lock. An agent should use the `etag` returned by `show --json` with
`update --if-match` when it has read a task that another agent may change.

```sh
work-ledger claim wl-01... --agent codex/research
work-ledger note wl-01... "Confirmed the migration boundary" --agent codex/research
work-ledger close wl-01... "Moved the endpoint and verified the caller" --agent codex/research
```

Set `WORK_LEDGER_AGENT` to give local interactive commands a consistent default
agent name. An explicit `--agent` always takes precedence.
