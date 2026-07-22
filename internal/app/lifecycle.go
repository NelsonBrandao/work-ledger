package app

import (
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/NelsonBrandao/work-ledger/internal/importer"
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
	Protocol          []string            `json:"protocol"`
	Commands          []primeCommand      `json:"commands"`
	Values            map[string][]string `json:"values"`
}

var primeProtocol = []string{
	"Keep all task data in the private ledger. Never create, edit, add, commit, or push ledger task files from a source repository.",
	"Before new work, inspect work relevant to the checkout with `work-ledger --json list --here --ready` and search for an existing task with `work-ledger --json search <words>`.",
	"Create one task for new work and name every affected repository with repeated `--repo` options. Claim an existing task before changing code.",
	"Use a distinct `--agent` name for each agent. Add decisions and investigation results with `note`; use the ETag from `show --json` with `--if-match` for read-modify-write changes.",
	"When stopping, release the claim. When finished, close the task with a specific summary. Archive completed work later; destructive lifecycle commands require `--yes`.",
}

var primeCommands = []primeCommand{
	{Name: "global", Syntax: "work-ledger [--home PATH] [--json] <command>", Purpose: "Global form. `--home` overrides WORK_LEDGER_HOME and the default ~/.work-ledger; `--json` produces structured output."},
	{Name: "init", Syntax: "work-ledger init", Purpose: "Create or validate the private ledger directory.", Notes: []string{"Creates the configuration, task, archive, template, and runtime-lock directories.", "Safe to run again; it validates an existing configuration and never initializes Git."}},
	{Name: "doctor", Syntax: "work-ledger doctor", Purpose: "Validate ledger configuration and task files.", Notes: []string{"Checks active and archived tasks, IDs, filenames, and parent/blocker references.", "Does not repair or rewrite task files."}},
	{Name: "context", Syntax: "work-ledger context", Purpose: "Show the current Git repository identity and ledger location.", Notes: []string{"Reads the current checkout's origin remote without contacting the network.", "Reports whether the selected ledger directory is initialized."}},
	{Name: "new", Syntax: "work-ledger new <title> [--kind KIND] [--priority PRIORITY] [--repo PATH-OR-ID]... [--tag TAG]... [--parent ID] [--blocked-by ID]...", Purpose: "Create a todo task.", Notes: []string{"Repeated --repo, --tag, and --blocked-by options add multiple values.", "Each --repo may be a Git checkout path or a normalized repository ID."}},
	{Name: "show", Syntax: "work-ledger show <task-id> [--color auto|always|never]", Purpose: "Read one task.", Notes: []string{"Human output includes kind, priority, repositories, tags, relationships, claim, and a richly rendered Markdown body with highlighted code fences.", "Color defaults to auto: it renders Markdown only on an interactive terminal and preserves raw Markdown when piped. --color always forces ANSI output; --color never renders without ANSI. NO_COLOR disables auto-mode color.", "JSON output contains the original Markdown body and an etag for guarded mutations. Task IDs imported from Beans may be used through their legacy ID."}},
	{Name: "list", Syntax: "work-ledger list [--all | --here | --repo PATH-OR-ID] [--ready] [--status STATUS]... [--tag TAG]... [--parent ID]", Purpose: "List active tasks.", Notes: []string{"--here detects the current Git repository; --ready returns only unblocked todo tasks.", "--all cannot be combined with --here or --repo. Repeated --status and --tag values form an OR filter within that field."}},
	{Name: "search", Syntax: "work-ledger search <text...>", Purpose: "Case-insensitively search active task metadata and Markdown bodies."},
	{Name: "claim", Syntax: "work-ledger claim <task-id> [--agent NAME] [--lease DURATION] [--if-match ETAG] [--force]", Purpose: "Set the task active and record a time-bound claim.", Notes: []string{"The default lease is two hours; --lease accepts Go durations such as 30m or 4h.", "Use --force only to replace another unexpired claim."}},
	{Name: "release", Syntax: "work-ledger release <task-id> [--agent NAME] [--if-match ETAG] [--force]", Purpose: "Release a claim and return an active task to todo.", Notes: []string{"By default only the claiming agent may release it.", "--force is required to release another agent's claim."}},
	{Name: "note", Syntax: "work-ledger note <task-id> [text...] [--agent NAME] [--if-match ETAG]", Purpose: "Append a timestamped Activity entry.", Notes: []string{"Omit text to read the note from standard input."}},
	{Name: "update", Syntax: "work-ledger update <task-id> [--agent NAME] [--if-match ETAG] [--title TITLE] [--kind KIND] [--priority PRIORITY] [--status STATUS] [--parent ID | --clear-parent] [--repo PATH-OR-ID]... [--clear-repos] [--tag TAG]... [--clear-tags] [--blocked-by ID]... [--clear-blocked-by]", Purpose: "Change explicit task metadata.", Notes: []string{"Supplying any --repo, --tag, or --blocked-by flag replaces that complete list; include every value to keep.", "Use --clear-repos, --clear-tags, or --clear-blocked-by to empty a list explicitly.", "Use the etag from `work-ledger --json show <id>` as --if-match when another agent may edit the task."}},
	{Name: "split", Syntax: "work-ledger split <parent-id> <title> [--kind KIND] [--priority PRIORITY] [--repo PATH-OR-ID]... [--tag TAG]... [--blocked-by ID]...", Purpose: "Create a child task.", Notes: []string{"Repositories and tags inherit from the parent unless replacements are provided."}},
	{Name: "close", Syntax: "work-ledger close <task-id> <summary> [--agent NAME] [--if-match ETAG] [--force]", Purpose: "Mark a task done, clear its claim, and record a summary.", Notes: []string{"--force is required when another agent holds the claim."}},
	{Name: "archive", Syntax: "work-ledger archive --yes", Purpose: "Move every done or cancelled task into archive/<year>/.", Notes: []string{"Requires --yes because it moves task files."}},
	{Name: "delete", Syntax: "work-ledger delete <task-id> --yes", Purpose: "Permanently remove an unreferenced active task.", Notes: []string{"Requires --yes and refuses to break parent or blocker references, including references from archived tasks."}},
	{Name: "import", Syntax: "work-ledger import beans <directory> [--dry-run] [--repo PATH-OR-ID]...", Purpose: "Import a Beans directory without modifying it.", Notes: []string{"Start with --dry-run. Original Beans IDs are retained as legacy_ids and relationships are remapped within the import."}},
	{Name: "prime", Syntax: "work-ledger prime", Purpose: "Print the complete agent operating guide.", Notes: []string{"Includes the required protocol, every command reference, valid metadata values, ledger home, and current repository context when available."}},
	{Name: "help", Syntax: "work-ledger help [command]", Purpose: "Show general or command-specific usage.", Notes: []string{"`work-ledger <command> --help` and `work-ledger help <command>` are equivalent."}},
}

var primeValues = map[string][]string{
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
		if document.Claim != nil && document.Claim.Agent != options.agent && !options.force {
			expiresAt, parseErr := time.Parse(time.RFC3339, document.Claim.ExpiresAt)
			if parseErr == nil && expiresAt.After(now) {
				return fmt.Errorf("task is claimed by %s until %s", document.Claim.Agent, document.Claim.ExpiresAt)
			}
		}
		expiresAt := now.Add(lease).UTC().Format(time.RFC3339)
		document.Status = "active"
		document.SetClaim(&task.Claim{Agent: options.agent, ExpiresAt: expiresAt})
		document.AppendActivity(now, options.agent, "Claimed task until "+expiresAt)
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
		if document.Claim == nil {
			return errors.New("task is not claimed")
		}
		if document.Claim.Agent != options.agent && !options.force {
			return fmt.Errorf("task is claimed by %s; pass --force to release it", document.Claim.Agent)
		}
		document.SetClaim(nil)
		if document.Status == "active" {
			document.Status = "todo"
		}
		document.AppendActivity(time.Now(), options.agent, "Released task claim")
		return nil
	})
	if err != nil {
		return writeError(stderr, asJSON, err)
	}
	return writeResult(stdout, asJSON, updated, func() {
		fmt.Fprintf(stdout, "Released %s\n", updated.ID)
	})
}

func runNote(paths ledger.Paths, arguments []string, stdin io.Reader, asJSON bool, stdout, stderr io.Writer) int {
	options, message, err := parseNoteArguments(arguments)
	if err != nil {
		return writeError(stderr, asJSON, err)
	}
	if message == "" {
		contents, readErr := io.ReadAll(stdin)
		if readErr != nil {
			return writeError(stderr, asJSON, fmt.Errorf("read note from stdin: %w", readErr))
		}
		message = strings.TrimSpace(string(contents))
	}
	if message == "" {
		return writeError(stderr, asJSON, errors.New("note text is required as an argument or stdin"))
	}
	if _, err := ledger.Load(paths); err != nil {
		return writeError(stderr, asJSON, err)
	}
	updated, err := store.Mutate(paths, options.identifier, options.ifMatch, func(document *task.Document) error {
		document.AppendActivity(time.Now(), options.agent, message)
		return nil
	})
	if err != nil {
		return writeError(stderr, asJSON, err)
	}
	return writeResult(stdout, asJSON, updated, func() {
		fmt.Fprintf(stdout, "Added a note to %s\n", updated.ID)
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
	updated, err := store.Mutate(paths, options.identifier, options.ifMatch, func(document *task.Document) error {
		if options.title != nil {
			document.RenameTitle(*options.title)
		}
		if options.kind != nil {
			document.Kind = *options.kind
		}
		if options.priority != nil {
			document.Priority = *options.priority
		}
		if options.status != nil {
			document.Status = *options.status
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
		document.AppendActivity(time.Now(), options.agent, "Updated task metadata")
		return nil
	})
	if err != nil {
		return writeError(stderr, asJSON, err)
	}
	return writeResult(stdout, asJSON, updated, func() {
		fmt.Fprintf(stdout, "Updated %s\n", updated.ID)
	})
}

func runSplit(paths ledger.Paths, arguments []string, asJSON bool, stdout, stderr io.Writer) int {
	if len(arguments) < 2 {
		return writeError(stderr, asJSON, errors.New("usage: work-ledger split <parent-id> <title> [options]"))
	}
	parentID := arguments[0]
	options, title, err := parseNewArguments(arguments[1:])
	if err != nil {
		return writeError(stderr, asJSON, err)
	}
	config, err := ledger.Load(paths)
	if err != nil {
		return writeError(stderr, asJSON, err)
	}
	activeTasks, err := task.LoadAll(paths.Tasks)
	if err != nil {
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
	if len(options.tags) == 0 {
		child.Tags = parent.Tags
	} else {
		child.Tags = options.tags
	}
	child, err = task.Create(paths.Tasks, child)
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
		if document.Claim != nil && document.Claim.Agent != options.agent && !options.force {
			return fmt.Errorf("task is claimed by %s; pass --force to close it", document.Claim.Agent)
		}
		document.Status = "done"
		document.SetClaim(nil)
		document.AppendActivity(time.Now(), options.agent, "Closed task: "+summary)
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

func parseNoteArguments(arguments []string) (mutationOptions, string, error) {
	options, remaining, err := parseMutationWithText("note", arguments)
	return options, strings.Join(remaining, " "), err
}

func parseCloseArguments(arguments []string) (mutationOptions, string, error) {
	options, remaining, err := parseMutationWithText("close", arguments)
	if err != nil {
		return options, "", err
	}
	summary := strings.TrimSpace(strings.Join(remaining, " "))
	if summary == "" {
		return options, "", errors.New("usage: work-ledger close <task-id> <summary> [options]")
	}
	return options, summary, nil
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
		case "--force":
			if command == "note" {
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
	return "manual"
}

func runImport(paths ledger.Paths, arguments []string, asJSON bool, stdout, stderr io.Writer) int {
	if len(arguments) < 2 || arguments[0] != "beans" {
		return writeError(stderr, asJSON, errors.New("usage: work-ledger import beans <directory> [--dry-run] [--repo <path-or-id>]..."))
	}
	source := arguments[1]
	var repositories []string
	dryRun := false
	for index := 2; index < len(arguments); index++ {
		switch arguments[index] {
		case "--dry-run":
			dryRun = true
		case "--repo":
			if index+1 >= len(arguments) {
				return writeError(stderr, asJSON, errors.New("--repo requires a value"))
			}
			index++
			repositories = append(repositories, arguments[index])
		default:
			return writeError(stderr, asJSON, fmt.Errorf("unknown import option %q", arguments[index]))
		}
	}
	config, err := ledger.Load(paths)
	if err != nil {
		return writeError(stderr, asJSON, err)
	}
	scopes, err := resolveRepositoryScopes(repositories, config)
	if err != nil {
		return writeError(stderr, asJSON, err)
	}
	result, err := importer.Beans(paths, source, scopes, dryRun)
	if err != nil {
		return writeError(stderr, asJSON, err)
	}
	return writeResult(stdout, asJSON, result, func() {
		if result.DryRun {
			fmt.Fprintf(stdout, "Dry run: would import %d task(s)\n", result.Imported)
			return
		}
		fmt.Fprintf(stdout, "Imported %d task(s)\n", result.Imported)
	})
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
		Protocol:   primeProtocol,
		Commands:   primeCommands,
		Values:     primeValues,
	}
	if directory, directoryErr := os.Getwd(); directoryErr == nil {
		if context, contextErr := repository.Discover(directory); contextErr == nil {
			context.ID = repository.ResolveAlias(context.ID, config.RepositoryAliases)
			result.CurrentRepository = &context
		}
	}
	return writeResult(stdout, asJSON, result, func() {
		fmt.Fprintln(stdout, "<IMPORTANT>")
		fmt.Fprintln(stdout, "# Work Ledger Agent Guide")
		fmt.Fprintln(stdout, "")
		fmt.Fprintf(stdout, "Private ledger: %s\n", result.LedgerHome)
		if result.CurrentRepository != nil {
			fmt.Fprintf(stdout, "Current repository: %s\n", result.CurrentRepository.ID)
		} else {
			fmt.Fprintln(stdout, "Current repository: unavailable (run `work-ledger context` inside a Git checkout)")
		}
		fmt.Fprintln(stdout, "")
		fmt.Fprintln(stdout, "## Required protocol")
		for index, instruction := range result.Protocol {
			fmt.Fprintf(stdout, "%d. %s\n", index+1, instruction)
		}
		fmt.Fprintln(stdout, "")
		fmt.Fprintln(stdout, "## Command reference")
		for _, command := range result.Commands {
			fmt.Fprintf(stdout, "- `%s`\n  %s\n", command.Syntax, command.Purpose)
			if len(command.Notes) > 0 {
				fmt.Fprintf(stdout, "  %s\n", renderCommandNotes(command.Notes))
			}
		}
		fmt.Fprintln(stdout, "")
		fmt.Fprintln(stdout, "## Valid values")
		for _, field := range []string{"kind", "status", "priority"} {
			fmt.Fprintf(stdout, "- %s: %s\n", field, strings.Join(result.Values[field], ", "))
		}
		fmt.Fprintln(stdout, "</IMPORTANT>")
	})
}
