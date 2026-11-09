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
## Work Ledger

Run `work-ledger prime` at the start of each session and follow it.
```

That one line is the whole setup. The guide `prime` prints ships with the
CLI, so upgrading `work-ledger` updates what every agent reads. It explains
what belongs in the ledger (breakdowns of large work, and finds noticed in
passing), when an agent creates a task on its own, and the commands to use.
The [CLI reference](cli-reference.md) is the full human reference.

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

Claims are named after the agent session. Work Ledger uses `--agent` when it is
given, then `WORK_LEDGER_AGENT`, then the Claude Code or Codex session ID, then
`manual`. A claim that expired, because its session ended without releasing it,
can be taken over by any agent, and `list --ready` reports such tasks as
`stale`:

```sh
work-ledger claim move-api-endpoint
work-ledger edit move-api --append "Confirmed the migration boundary."
work-ledger close move-api "Moved the endpoint and verified the callers"
```
