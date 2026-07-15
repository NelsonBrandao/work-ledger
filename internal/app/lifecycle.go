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
		case "--force":
			options.force = true
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
	if _, err := ledger.Load(paths); err != nil {
		return writeError(stderr, asJSON, err)
	}
	instructions := []string{
		"Use work-ledger for non-trivial work tracking; it never writes to the source repository.",
		"Run `work-ledger list --here --ready --json` to find relevant available work.",
		"Create or claim a task before making changes, and use `work-ledger note` for decisions and investigation results.",
		"Close completed work with `work-ledger close <id> <summary>`; archive it later with `work-ledger archive --yes`.",
		"Use the etag from `work-ledger show <id> --json` as `work-ledger update --if-match` when another agent may edit the task.",
	}
	return writeResult(stdout, asJSON, map[string]any{"ledger_home": paths.Home, "instructions": instructions}, func() {
		for _, instruction := range instructions {
			fmt.Fprintln(stdout, instruction)
		}
	})
}
