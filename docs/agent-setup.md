# Set up Work Ledger for Codex and Claude Code

This is a one-time, personal-machine setup. You install one local binary,
create one private ledger directory, and tell each local agent where to find
the workflow instructions.

You do **not** add the ledger, its tasks, or these personal instructions to a
source repository. Repository `AGENTS.md` and `CLAUDE.md` files should stay
focused on team-shared codebase rules.

## What you are configuring

```text
Your machine
  ~/.codex/AGENTS.md       Personal instructions for Codex
  ~/.claude/CLAUDE.md      Personal instructions for Claude Code
  ~/.work-ledger/          Private task files, configuration, and locks
  work-ledger              CLI available on PATH
```

This setup works with agents that can use your local shell and filesystem:
Codex CLI, local Codex IDE/app sessions, and local Claude Code sessions. A
cloud or remote agent does not automatically have access to your local ledger;
it needs the binary and the private ledger directory in its own environment.

## 1. Install and initialize the CLI

Install the released CLI without cloning a checkout:

```sh
go env -w GOPRIVATE=github.com/NelsonBrandao/*
go install github.com/NelsonBrandao/work-ledger/cmd/work-ledger@v0.5.0
```

Ensure `$GOBIN` (or `$(go env GOPATH)/bin` when `GOBIN` is unset) is on your
`PATH`, then create and validate the private ledger:

```sh
work-ledger init
work-ledger doctor
```

The default location is `~/.work-ledger`. It is the simplest choice because
both Codex and Claude Code inherit it without extra configuration. Existing
ledgers are not moved automatically; set `WORK_LEDGER_HOME=~/work-ledger` to
continue using one at the former default location. To use a different location,
set `WORK_LEDGER_HOME` in the environment that launches the agent:

```sh
export WORK_LEDGER_HOME="$HOME/Library/Application Support/work-ledger"
work-ledger init
```

## 2. Add the shared instruction block

Append the following block to the personal-instructions file for each agent in
the next sections. Do not replace existing personal instructions.

```md
## Private work tracking with Work Ledger

For every non-trivial engineering task on this machine:

1. Run `work-ledger prime` before acting; it contains the full command
   reference and operating protocol.
2. Run `work-ledger --json list --here --ready` in a Git checkout, then
   `work-ledger --json search "<relevant words>"` before creating duplicate
   work.
3. Create one task for new work, including every affected repository with
   `--repo`. Claim an existing task before changing code.
4. Record material decisions and investigation results with `work-ledger note`.
   When updating a task after reading it, take the `etag` from
   `work-ledger --json show <id>` and pass it as `--if-match`.
5. Release the claim when stopping, or close the task with a useful summary
   when finished. Do not archive or delete without the command's explicit
   `--yes` confirmation.

Work Ledger data is private local state. Never create, edit, add, commit, or
push ledger task files from a source repository.
```

The [CLI reference](cli-reference.md) is the durable human reference. `prime`
contains the same agent-oriented operating detail directly in the terminal.

## 3. Codex setup

Codex reads global personal instructions from `~/.codex/AGENTS.md` at the start
of each session. This file layers with a repository's own `AGENTS.md`; the
repository file remains responsible for shared project rules.

1. Open your global instructions file:

   ```sh
   mkdir -p ~/.codex
   $EDITOR ~/.codex/AGENTS.md
   ```

2. Paste the shared instruction block above at the end, then save it.

3. Start a **new** Codex session from any repository. Codex builds its
   instruction chain when the session starts, so an already-running session
   will not see the change automatically.

4. Verify it with a prompt such as: “Summarize my personal Work Ledger
   workflow before you start.” The response should mention `work-ledger prime`
   and that task files must remain outside the repository.

If `~/.codex/AGENTS.override.md` exists, Codex uses that file instead of
`~/.codex/AGENTS.md` at the global level. Add the block to the override file or
remove the temporary override when you want the normal global instructions to
apply again.

## 4. Claude Code setup

Claude Code reads user-wide instructions from `~/.claude/CLAUDE.md`. They apply
to every project, while a repository's `CLAUDE.md` loads afterward for
project-specific conventions.

1. Open your user instructions file:

   ```sh
   mkdir -p ~/.claude
   $EDITOR ~/.claude/CLAUDE.md
   ```

2. Paste the shared instruction block above at the end, then save it.

3. Start a new Claude Code session in a repository:

   ```sh
   cd ~/code/some-repository
   claude
   ```

4. Run `/memory` in Claude Code to see the loaded instruction files, then ask:
   “Summarize my personal Work Ledger workflow before you start.”

Avoid using `CLAUDE.local.md` for this workflow: it is a personal,
project-specific file and will not follow you across every repository or
worktree. Do not add this block to a project `CLAUDE.md` unless the whole team
intentionally shares the same tracker.

## 5. Verify the ledger itself

From a Git checkout that has an `origin` remote, run:

```sh
work-ledger context
work-ledger --json list --here --ready
```

`context` should show the private ledger home and the repository's normalized
identity. Create a harmless test task if you want to prove multi-repository
discovery:

```sh
work-ledger new "Verify Work Ledger setup" --repo . --tag setup
work-ledger list --here --ready
```

Close or delete this test task once the setup is confirmed.

## Working across repositories and agents

Create one task with all repositories it changes:

```sh
work-ledger new "Move API endpoint" --repo ~/code/parcel --repo ~/code/services
```

The CLI identifies checkouts from their normalized `origin` remotes. Use
`work-ledger context` to inspect the current identity, and configure fork
aliases in the private `ledger.yaml` file when needed.

For concurrent local agents, use a distinct agent name and an ETag when making
a read-modify-write change:

```sh
work-ledger claim wl-01... --agent codex/api-migration
work-ledger note wl-01... "Confirmed the migration boundary" --agent codex/api-migration
work-ledger close wl-01... "Moved the endpoint and verified the callers" --agent codex/api-migration
```

Set `WORK_LEDGER_AGENT` to choose a default identity for interactive commands;
an explicit `--agent` always takes precedence.
