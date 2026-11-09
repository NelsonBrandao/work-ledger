package app

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/NelsonBrandao/work-ledger/internal/ledger"
	"github.com/NelsonBrandao/work-ledger/internal/repository"
	"github.com/NelsonBrandao/work-ledger/internal/store"
	"github.com/NelsonBrandao/work-ledger/internal/task"
)

const defaultLease = 2 * time.Hour

type mutationOptions struct {
	identifier string
	agent      string
	ifMatch    string
	force      bool
}

type updateOptions struct {
	mutationOptions
	title           *string
	kind            *string
	priority        *string
	status          *string
	parent          *string
	repositories    []string
	tags            []string
	blockedBy       []string
	replaceTags     bool
	replaceBlockers bool
	replaceRepos    bool
	clearParent     bool
}

type primeCommand struct {
	Name    string   `json:"name"`
	Syntax  string   `json:"syntax"`
	Purpose string   `json:"purpose"`
	Notes   []string `json:"notes,omitempty"`
}

type primeResult struct {
	LedgerHome        string              `json:"ledger_home"`
	CurrentRepository *repository.Context `json:"current_repository,omitempty"`
	Guide             string              `json:"guide"`
	Commands          []string            `json:"commands"`
}

// primeGuide is what an agent reads at the start of a session. Keep it short:
// details belong in `work-ledger help <command>`.
const primeGuide = `Work Ledger is a private backlog on this machine. It holds two kinds of work
that do not belong in the team's issue tracker:

- Breakdowns: a large piece of work split into child tasks, so separate
  sessions can take them one at a time or in parallel.
- Finds: something noticed in passing that is worth doing later, such as an
  odd log line, a latent bug, a refactor, or one piece of a bigger problem.

When to create a task:
- When you notice a find outside the current request, create it yourself
  and mention its ID to the user in one line.
- When work is too large for one session, propose a breakdown, and create the
  parent and its children once the user agrees.
- Do not create a task for the request you are working on now, or for work
  the issue tracker already holds.
- Search first: ` + "`work-ledger search <words>`" + `.

Working on a task:
- Claim it before you change code, close it with the outcome when you finish,
  and release it if you stop without finishing.
- Keep the description current. Rewrite it when what you know changes, or
  append a paragraph. Write what the next reader needs; do not keep a log.
- Task files live in the ledger home. Never copy them into a repository.`

var primeCommandLines = []string{
	"work-ledger new <title> [--description TEXT|-] [--repo PATH]... [--tag TAG]... [--parent ID] [--kind KIND] [--priority PRIORITY]",
	"work-ledger split <parent-id> <title> [same options as new]",
	"work-ledger list [--here] [--ready] [--status STATUS]... [--tag TAG]... [--parent ID]",
	"work-ledger search <words>",
	"work-ledger show <id>",
	"work-ledger edit <id> [--append] [TEXT]   (TEXT from stdin when omitted)",
	"work-ledger claim <id> | release <id> | close <id> [outcome]",
	"work-ledger update <id> [--title T] [--status S] [--priority P] [--tag T]... [--parent ID] [--blocked-by ID]...",
}

var primeCommands = []primeCommand{
	{Name: "global", Syntax: "work-ledger [--home PATH] [--json] <command>", Purpose: "Global form. `--home` overrides WORK_LEDGER_HOME and the default ~/.work-ledger; `--json` produces structured output."},
	{Name: "init", Syntax: "work-ledger init", Purpose: "Create or validate the private ledger directory.", Notes: []string{"Creates the configuration, task, archive, template, and runtime-lock directories.", "Safe to run again; it validates an existing configuration and never initializes Git."}},
	{Name: "doctor", Syntax: "work-ledger doctor", Purpose: "Validate ledger configuration and task files.", Notes: []string{"Checks active and archived tasks, IDs, filenames, and parent/blocker references.", "Does not repair or rewrite task files."}},
	{Name: "context", Syntax: "work-ledger context", Purpose: "Show the current Git repository identity and ledger location.", Notes: []string{"Reads the current checkout's origin remote without contacting the network.", "Reports whether the selected ledger directory is initialized."}},
	{Name: "new", Syntax: "work-ledger new <title> [--description TEXT|-] [--kind KIND] [--priority PRIORITY] [--repo PATH-OR-ID]... [--tag TAG]... [--parent ID] [--blocked-by ID]...", Purpose: "Create a todo task.", Notes: []string{"The ID is the title as a short slug, such as fix-flaky-login-test; a four-character suffix is added only when that ID is taken.", "--description - reads the description from standard input. --title is accepted in place of the positional title.", "Repeated --repo, --tag, and --blocked-by options add multiple values.", "Each --repo may be a Git checkout path or a normalized repository ID.", "kind: " + strings.Join(validValues["kind"], ", ") + "; priority: " + strings.Join(validValues["priority"], ", ") + "."}},
	{Name: "show", Syntax: "work-ledger show <task-id> [--color auto|always|never]", Purpose: "Read one task.", Notes: []string{"Every command that takes a task ID accepts a unique prefix of it, or a legacy ID.", "Human output includes kind, priority, repositories, tags, relationships, claim, and the rendered Markdown description.", "Color defaults to auto: it renders Markdown only on an interactive terminal and preserves raw Markdown when piped. --color always forces ANSI output; --color never renders without ANSI. NO_COLOR disables auto-mode color.", "JSON output contains the original Markdown body and an etag for guarded mutations."}},
	{Name: "list", Syntax: "work-ledger list [--all | --here | --repo PATH-OR-ID] [--ready] [--status STATUS]... [--tag TAG]... [--parent ID]", Purpose: "List active tasks.", Notes: []string{"--here detects the current Git repository; --ready returns unblocked todo tasks and stale ones.", "A task is stale when it is active but its claim expired: the session that held it stopped without releasing or closing it.", "--all cannot be combined with --here or --repo. Repeated --status and --tag values form an OR filter within that field."}},
	{Name: "search", Syntax: "work-ledger search <text...>", Purpose: "Case-insensitively search active task metadata and descriptions."},
	{Name: "edit", Syntax: "work-ledger edit <task-id> [--append] [text...] [--if-match ETAG]", Purpose: "Rewrite or extend a task's description.", Notes: []string{"Without --append the text replaces the whole description; it gets the title as its heading unless it starts with its own `# ` heading.", "--append adds the text as a new paragraph at the end.", "Omit the text to read it from standard input.", "Pass the etag from `work-ledger --json show <id>` as --if-match to refuse the edit when the task changed since you read it."}},
	{Name: "note", Syntax: "work-ledger note <task-id> [text...]", Purpose: "Alias for `work-ledger edit <task-id> --append`, kept for older instructions."},
	{Name: "claim", Syntax: "work-ledger claim <task-id> [--agent NAME] [--lease DURATION] [--if-match ETAG] [--force]", Purpose: "Set the task active and record a time-bound claim.", Notes: []string{"The default lease is two hours; --lease accepts Go durations such as 30m or 4h.", "The agent name defaults to WORK_LEDGER_AGENT, then to the Claude Code or Codex session, then to manual.", "An expired claim can be taken over; --force replaces one that has not expired."}},
	{Name: "release", Syntax: "work-ledger release <task-id> [--agent NAME] [--if-match ETAG] [--force]", Purpose: "Release a claim and return an active task to todo.", Notes: []string{"Anyone may release an expired claim; --force is required to release another agent's unexpired claim."}},
	{Name: "update", Syntax: "work-ledger update <task-id> [--agent NAME] [--if-match ETAG] [--title TITLE] [--kind KIND] [--priority PRIORITY] [--status STATUS] [--parent ID | --clear-parent] [--repo PATH-OR-ID]... [--clear-repos] [--tag TAG]... [--clear-tags] [--blocked-by ID]... [--clear-blocked-by]", Purpose: "Change explicit task metadata.", Notes: []string{"Supplying any --repo, --tag, or --blocked-by flag replaces that complete list; include every value to keep.", "Use --clear-repos, --clear-tags, or --clear-blocked-by to empty a list explicitly.", "status: " + strings.Join(validValues["status"], ", ") + ".", "Use the etag from `work-ledger --json show <id>` as --if-match when another agent may edit the task."}},
	{Name: "split", Syntax: "work-ledger split <parent-id> <title> [--description TEXT|-] [--kind KIND] [--priority PRIORITY] [--repo PATH-OR-ID]... [--tag TAG]... [--blocked-by ID]...", Purpose: "Create a child task.", Notes: []string{"Repositories and tags inherit from the parent unless replacements are provided."}},
	{Name: "close", Syntax: "work-ledger close <task-id> [outcome] [--agent NAME] [--if-match ETAG] [--force]", Purpose: "Mark a task done, clear its claim, and append the outcome to the description.", Notes: []string{"--force is required when another agent holds an unexpired claim."}},
	{Name: "archive", Syntax: "work-ledger archive --yes", Purpose: "Move every done or cancelled task into archive/<year>/.", Notes: []string{"Requires --yes because it moves task files."}},
	{Name: "delete", Syntax: "work-ledger delete <task-id> --yes", Purpose: "Permanently remove an unreferenced active task.", Notes: []string{"Requires --yes and refuses to break parent or blocker references, including references from archived tasks."}},
	{Name: "prime", Syntax: "work-ledger prime", Purpose: "Print the short agent guide: when to create tasks, how to work them, and the common commands.", Notes: []string{"Point an agent's personal instructions at this command; the guide ships with the CLI, so it changes with each release."}},
	{Name: "help", Syntax: "work-ledger help [command]", Purpose: "Show general or command-specific usage.", Notes: []string{"`work-ledger <command> --help` and `work-ledger help <command>` are equivalent."}},
}

var validValues = map[string][]string{
	"kind":     {"task", "bug", "investigation", "feature"},
	"status":   {"todo", "active", "blocked", "done", "cancelled"},
	"priority": {"critical", "high", "normal", "low"},
}

func runClaim(paths ledger.Paths, arguments []string, asJSON bool, stdout, stderr io.Writer) int {
	options, lease, err := parseClaimArguments(arguments)
	if err != nil {
		return writeError(stderr, asJSON, err)
	}
	if _, err := ledger.Load(paths); err != nil {
		return writeError(stderr, asJSON, err)
	}
	now := time.Now()
	updated, err := store.Mutate(paths, options.identifier, options.ifMatch, func(document *task.Document) error {
		if err := checkClaim(document, options, now, "claim"); err != nil {
			return err
		}
		expiresAt := now.Add(lease).UTC().Format(time.RFC3339)
		document.Status = "active"
		document.SetClaim(&task.Claim{Agent: options.agent, ExpiresAt: expiresAt})
		return nil
	})
	if err != nil {
		return writeError(stderr, asJSON, err)
	}
	return writeResult(stdout, asJSON, updated, func() {
		fmt.Fprintf(stdout, "Claimed %s until %s\n", updated.ID, updated.Claim.ExpiresAt)
	})
}

func runRelease(paths ledger.Paths, arguments []string, asJSON bool, stdout, stderr io.Writer) int {
	options, err := parseMutationArguments("release", arguments)
	if err != nil {
		return writeError(stderr, asJSON, err)
	}
	if _, err := ledger.Load(paths); err != nil {
		return writeError(stderr, asJSON, err)
	}
	updated, err := store.Mutate(paths, options.identifier, options.ifMatch, func(document *task.Document) error {
		if document.Claim == nil && document.Status != "active" {
			return errors.New("task is not claimed")
		}
		if err := checkClaim(document, options, time.Now(), "release"); err != nil {
			return err
		}
		document.SetClaim(nil)
		if document.Status == "active" {
			document.Status = "todo"
		}
		return nil
	})
	if err != nil {
		return writeError(stderr, asJSON, err)
	}
	return writeResult(stdout, asJSON, updated, func() {
		fmt.Fprintf(stdout, "Released %s\n", updated.ID)
	})
}

func runEdit(paths ledger.Paths, arguments []string, stdin io.Reader, appendOnly bool, asJSON bool, stdout, stderr io.Writer) int {
	options, remaining, err := parseMutationWithText("edit", arguments)
	if err != nil {
		return writeError(stderr, asJSON, err)
	}
	var words []string
	for _, argument := range remaining {
		if argument == "--append" {
			appendOnly = true
			continue
		}
		words = append(words, argument)
	}
	text := strings.Join(words, " ")
	if text == "" || text == "-" {
		contents, readErr := io.ReadAll(stdin)
		if readErr != nil {
			return writeError(stderr, asJSON, fmt.Errorf("read description from stdin: %w", readErr))
		}
		text = string(contents)
	}
	if strings.TrimSpace(text) == "" {
		return writeError(stderr, asJSON, errors.New("description text is required as an argument or stdin"))
	}
	if _, err := ledger.Load(paths); err != nil {
		return writeError(stderr, asJSON, err)
	}
	updated, err := store.Mutate(paths, options.identifier, options.ifMatch, func(document *task.Document) error {
		if appendOnly {
			document.AppendDescription(text)
		} else {
			document.ReplaceDescription(text)
		}
		return nil
	})
	if err != nil {
		return writeError(stderr, asJSON, err)
	}
	return writeResult(stdout, asJSON, updated, func() {
		fmt.Fprintf(stdout, "Updated the description of %s\n", updated.ID)
	})
}

func runUpdate(paths ledger.Paths, arguments []string, asJSON bool, stdout, stderr io.Writer) int {
	options, err := parseUpdateArguments(arguments)
	if err != nil {
		return writeError(stderr, asJSON, err)
	}
	config, err := ledger.Load(paths)
	if err != nil {
		return writeError(stderr, asJSON, err)
	}
	repositories, err := resolveRepositoryScopes(options.repositories, config)
	if err != nil {
		return writeError(stderr, asJSON, err)
	}
	if options.parent != nil {
		parentIDs, err := resolveTaskIDs(paths, []string{*options.parent})
		if err != nil {
			return writeError(stderr, asJSON, err)
		}
		options.parent = &parentIDs[0]
	}
	if options.blockedBy, err = resolveTaskIDs(paths, options.blockedBy); err != nil {
		return writeError(stderr, asJSON, err)
	}
	updated, err := store.Mutate(paths, options.identifier, options.ifMatch, func(document *task.Document) error {
		if options.title != nil {
			document.RenameTitle(*options.title)
		}
		if options.kind != nil {
			document.Kind = *options.kind
		}
		if options.priority != nil {
			document.Priority = normalizePriority(*options.priority)
		}
		if options.status != nil {
			document.Status = normalizeStatus(*options.status)
		}
		if options.clearParent {
			document.Parent = ""
		} else if options.parent != nil {
			document.Parent = *options.parent
		}
		if options.replaceTags {
			document.Tags = options.tags
		}
		if options.replaceBlockers {
			document.BlockedBy = options.blockedBy
		}
		if options.replaceRepos {
			document.Repositories = repositories
		}
		return nil
	})
	if err != nil {
		return writeError(stderr, asJSON, err)
	}
	return writeResult(stdout, asJSON, updated, func() {
		fmt.Fprintf(stdout, "Updated %s\n", updated.ID)
	})
}

func runSplit(paths ledger.Paths, arguments []string, stdin io.Reader, asJSON bool, stdout, stderr io.Writer) int {
	if len(arguments) < 2 {
		return writeError(stderr, asJSON, errors.New("usage: work-ledger split <parent-id> <title> [options]"))
	}
	parentID := arguments[0]
	options, title, err := parseNewArguments(arguments[1:], stdin)
	if err != nil {
		return writeError(stderr, asJSON, err)
	}
	config, err := ledger.Load(paths)
	if err != nil {
		return writeError(stderr, asJSON, err)
	}
	allTasks, err := task.LoadAll(paths.Tasks, paths.Archive)
	if err != nil {
		return writeError(stderr, asJSON, err)
	}
	activeTasks, err := task.LoadAll(paths.Tasks)
	if err != nil {
		return writeError(stderr, asJSON, err)
	}
	if options.blockedBy, err = findTaskIDs(allTasks, options.blockedBy); err != nil {
		return writeError(stderr, asJSON, err)
	}
	parent, err := task.Find(activeTasks, parentID)
	if err != nil {
		return writeError(stderr, asJSON, err)
	}
	repositories, err := resolveRepositoryScopes(options.repositories, config)
	if err != nil {
		return writeError(stderr, asJSON, err)
	}
	if len(repositories) == 0 {
		repositories = parent.Repositories
	}
	child, err := task.New(title, options.kind, options.priority, repositories, time.Now())
	if err != nil {
		return writeError(stderr, asJSON, err)
	}
	child.Parent = parent.ID
	child.BlockedBy = options.blockedBy
	child.Body = options.description
	if len(options.tags) == 0 {
		child.Tags = parent.Tags
	} else {
		child.Tags = options.tags
	}
	child, err = task.Create(paths.Tasks, child, task.TakenIDs(allTasks))
	if err != nil {
		return writeError(stderr, asJSON, err)
	}
	return writeResult(stdout, asJSON, child.Summary(), func() {
		fmt.Fprintf(stdout, "Created child %s for %s\n", child.ID, parent.ID)
	})
}

func runClose(paths ledger.Paths, arguments []string, asJSON bool, stdout, stderr io.Writer) int {
	options, summary, err := parseCloseArguments(arguments)
	if err != nil {
		return writeError(stderr, asJSON, err)
	}
	if _, err := ledger.Load(paths); err != nil {
		return writeError(stderr, asJSON, err)
	}
	updated, err := store.Mutate(paths, options.identifier, options.ifMatch, func(document *task.Document) error {
		if err := checkClaim(document, options, time.Now(), "close"); err != nil {
			return err
		}
		document.Status = "done"
		document.SetClaim(nil)
		if summary != "" {
			document.AppendDescription("**Outcome:** " + summary)
		}
		return nil
	})
	if err != nil {
		return writeError(stderr, asJSON, err)
	}
	return writeResult(stdout, asJSON, updated, func() {
		fmt.Fprintf(stdout, "Closed %s\n", updated.ID)
	})
}

func runArchive(paths ledger.Paths, arguments []string, asJSON bool, stdout, stderr io.Writer) int {
	if len(arguments) != 1 || arguments[0] != "--yes" {
		return writeError(stderr, asJSON, errors.New("usage: work-ledger archive --yes"))
	}
	if _, err := ledger.Load(paths); err != nil {
		return writeError(stderr, asJSON, err)
	}
	archived, err := store.Archive(paths)
	if err != nil {
		return writeError(stderr, asJSON, err)
	}
	summaries := make([]task.Summary, 0, len(archived))
	for _, archivedTask := range archived {
		summaries = append(summaries, archivedTask.Summary())
	}
	return writeResult(stdout, asJSON, summaries, func() {
		fmt.Fprintf(stdout, "Archived %d task(s)\n", len(summaries))
	})
}

func runDelete(paths ledger.Paths, arguments []string, asJSON bool, stdout, stderr io.Writer) int {
	if len(arguments) != 2 || arguments[1] != "--yes" {
		return writeError(stderr, asJSON, errors.New("usage: work-ledger delete <task-id> --yes"))
	}
	if _, err := ledger.Load(paths); err != nil {
		return writeError(stderr, asJSON, err)
	}
	deleted, err := store.Delete(paths, arguments[0])
	if err != nil {
		return writeError(stderr, asJSON, err)
	}
	return writeResult(stdout, asJSON, deleted.Summary(), func() {
		fmt.Fprintf(stdout, "Deleted %s\n", deleted.ID)
	})
}

func parseMutationArguments(command string, arguments []string) (mutationOptions, error) {
	var options mutationOptions
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		switch argument {
		case "--agent", "--if-match":
			if index+1 >= len(arguments) {
				return options, fmt.Errorf("%s requires a value", argument)
			}
			index++
			if argument == "--agent" {
				options.agent = arguments[index]
			} else {
				options.ifMatch = arguments[index]
			}
		case "--force":
			options.force = true
		default:
			if strings.HasPrefix(argument, "-") {
				return options, fmt.Errorf("unknown %s option %q", command, argument)
			}
			if options.identifier != "" {
				return options, fmt.Errorf("usage: work-ledger %s <task-id> [options]", command)
			}
			options.identifier = argument
		}
	}
	if options.identifier == "" {
		return options, fmt.Errorf("usage: work-ledger %s <task-id> [options]", command)
	}
	options.agent = agentName(options.agent)
	return options, nil
}

func parseClaimArguments(arguments []string) (mutationOptions, time.Duration, error) {
	var leaseValue string
	remaining := make([]string, 0, len(arguments))
	for index := 0; index < len(arguments); index++ {
		if arguments[index] != "--lease" {
			remaining = append(remaining, arguments[index])
			continue
		}
		if index+1 >= len(arguments) {
			return mutationOptions{}, 0, errors.New("--lease requires a duration")
		}
		index++
		leaseValue = arguments[index]
	}
	options, err := parseMutationArguments("claim", remaining)
	if err != nil {
		return mutationOptions{}, 0, err
	}
	if leaseValue == "" {
		return options, defaultLease, nil
	}
	lease, err := time.ParseDuration(leaseValue)
	if err != nil || lease <= 0 {
		return mutationOptions{}, 0, fmt.Errorf("invalid lease duration %q", leaseValue)
	}
	return options, lease, nil
}

func parseCloseArguments(arguments []string) (mutationOptions, string, error) {
	options, remaining, err := parseMutationWithText("close", arguments)
	if err != nil {
		return options, "", err
	}
	return options, strings.TrimSpace(strings.Join(remaining, " ")), nil
}

func parseMutationWithText(command string, arguments []string) (mutationOptions, []string, error) {
	var options mutationOptions
	var remaining []string
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		switch argument {
		case "--agent", "--if-match":
			if index+1 >= len(arguments) {
				return options, nil, fmt.Errorf("%s requires a value", argument)
			}
			index++
			if argument == "--agent" {
				options.agent = arguments[index]
			} else {
				options.ifMatch = arguments[index]
			}
		case "--append":
			if command != "edit" {
				return options, nil, fmt.Errorf("unknown %s option %q", command, argument)
			}
			remaining = append(remaining, argument)
		case "--force":
			if command == "edit" {
				return options, nil, fmt.Errorf("unknown %s option %q", command, argument)
			}
			options.force = true
		default:
			if strings.HasPrefix(argument, "-") {
				return options, nil, fmt.Errorf("unknown %s option %q", command, argument)
			}
			if options.identifier == "" {
				options.identifier = argument
			} else {
				remaining = append(remaining, argument)
			}
		}
	}
	if options.identifier == "" {
		return options, nil, fmt.Errorf("usage: work-ledger %s <task-id> [text]", command)
	}
	options.agent = agentName(options.agent)
	return options, remaining, nil
}

func parseUpdateArguments(arguments []string) (updateOptions, error) {
	var options updateOptions
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		switch argument {
		case "--clear-parent":
			options.clearParent = true
		case "--clear-tags":
			options.replaceTags = true
			options.tags = nil
		case "--clear-blocked-by":
			options.replaceBlockers = true
			options.blockedBy = nil
		case "--clear-repos":
			options.replaceRepos = true
			options.repositories = nil
		case "--agent", "--if-match", "--title", "--kind", "--priority", "--status", "--parent", "--repo", "--tag", "--blocked-by":
			if index+1 >= len(arguments) {
				return options, fmt.Errorf("%s requires a value", argument)
			}
			index++
			value := arguments[index]
			switch argument {
			case "--agent":
				options.agent = value
			case "--if-match":
				options.ifMatch = value
			case "--title":
				options.title = &value
			case "--kind":
				options.kind = &value
			case "--priority":
				options.priority = &value
			case "--status":
				options.status = &value
			case "--parent":
				options.parent = &value
			case "--repo":
				options.replaceRepos = true
				options.repositories = append(options.repositories, value)
			case "--tag":
				options.replaceTags = true
				options.tags = append(options.tags, value)
			case "--blocked-by":
				options.replaceBlockers = true
				options.blockedBy = append(options.blockedBy, value)
			}
		default:
			if strings.HasPrefix(argument, "-") {
				return options, fmt.Errorf("unknown update option %q", argument)
			}
			if options.identifier != "" {
				return options, errors.New("usage: work-ledger update <task-id> [options]")
			}
			options.identifier = argument
		}
	}
	if options.identifier == "" {
		return options, errors.New("usage: work-ledger update <task-id> [options]")
	}
	if options.title == nil && options.kind == nil && options.priority == nil && options.status == nil && options.parent == nil && !options.clearParent && !options.replaceTags && !options.replaceBlockers && !options.replaceRepos {
		return options, errors.New("update requires at least one field change")
	}
	options.agent = agentName(options.agent)
	return options, nil
}

func agentName(value string) string {
	if value != "" {
		return value
	}
	if value = os.Getenv("WORK_LEDGER_AGENT"); value != "" {
		return value
	}
	for _, session := range []struct{ prefix, variable string }{
		{"claude", "CLAUDE_CODE_SESSION_ID"},
		{"codex", "CODEX_SESSION_ID"},
	} {
		if value = os.Getenv(session.variable); value != "" {
			if len(value) > 8 {
				value = value[:8]
			}
			return session.prefix + "-" + value
		}
	}
	return "manual"
}

// checkClaim refuses to act on a task another agent holds an unexpired claim
// on, unless --force was given.
func checkClaim(document *task.Document, options mutationOptions, now time.Time, command string) error {
	claim := document.Claim
	if claim == nil || claim.Agent == options.agent || options.force || !claim.Held(now) {
		return nil
	}
	return fmt.Errorf("task is claimed by %s until %s; pass --force to %s it", claim.Agent, claim.ExpiresAt, command)
}

func normalizePriority(value string) string {
	switch strings.ToLower(value) {
	case "medium", "med", "default":
		return "normal"
	case "urgent", "highest":
		return "critical"
	case "lowest":
		return "low"
	}
	return value
}

func normalizeStatus(value string) string {
	switch strings.ToLower(value) {
	case "in-progress", "in_progress", "doing", "started":
		return "active"
	case "closed", "complete", "completed", "finished":
		return "done"
	case "canceled", "dropped", "wontfix":
		return "cancelled"
	case "open", "new", "backlog":
		return "todo"
	}
	return value
}

func resolveTaskIDs(paths ledger.Paths, identifiers []string) ([]string, error) {
	if len(identifiers) == 0 {
		return identifiers, nil
	}
	tasks, err := task.LoadAll(paths.Tasks, paths.Archive)
	if err != nil {
		return nil, err
	}
	return findTaskIDs(tasks, identifiers)
}

func findTaskIDs(tasks []task.Task, identifiers []string) ([]string, error) {
	resolved := make([]string, 0, len(identifiers))
	for _, identifier := range identifiers {
		found, err := task.Find(tasks, identifier)
		if err != nil {
			return nil, err
		}
		resolved = append(resolved, found.ID)
	}
	return resolved, nil
}

func runPrime(paths ledger.Paths, arguments []string, asJSON bool, stdout, stderr io.Writer) int {
	if len(arguments) > 0 {
		return writeError(stderr, asJSON, errors.New("prime does not accept arguments"))
	}
	config, err := ledger.Load(paths)
	if err != nil {
		return writeError(stderr, asJSON, err)
	}
	result := primeResult{
		LedgerHome: paths.Home,
		Guide:      primeGuide,
		Commands:   primeCommandLines,
	}
	if directory, directoryErr := os.Getwd(); directoryErr == nil {
		if context, contextErr := repository.Discover(directory); contextErr == nil {
			context.ID = repository.ResolveAlias(context.ID, config.RepositoryAliases)
			result.CurrentRepository = &context
		}
	}
	return writeResult(stdout, asJSON, result, func() {
		fmt.Fprintln(stdout, "# Work Ledger")
		fmt.Fprintln(stdout, "")
		fmt.Fprintln(stdout, result.Guide)
		fmt.Fprintln(stdout, "")
		fmt.Fprintln(stdout, "Commands (task IDs accept any unique prefix; `work-ledger help <command>` has details):")
		for _, line := range result.Commands {
			fmt.Fprintf(stdout, "  %s\n", line)
		}
		fmt.Fprintln(stdout, "")
		fmt.Fprintf(stdout, "Ledger home: %s\n", result.LedgerHome)
		if result.CurrentRepository != nil {
			fmt.Fprintf(stdout, "Current repository: %s\n", result.CurrentRepository.ID)
		}
	})
}
