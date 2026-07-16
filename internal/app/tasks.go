package app

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/NelsonBrandao/work-ledger/internal/ledger"
	"github.com/NelsonBrandao/work-ledger/internal/repository"
	"github.com/NelsonBrandao/work-ledger/internal/task"
)

type newOptions struct {
	repositories []string
	tags         []string
	blockedBy    []string
	kind         string
	priority     string
	parent       string
}

type listOptions struct {
	repository string
	parent     string
	statuses   []string
	tags       []string
	here       bool
	ready      bool
	all        bool
}

func runNew(paths ledger.Paths, arguments []string, asJSON bool, stdout, stderr io.Writer) int {
	options, title, err := parseNewArguments(arguments)
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
	created, err := task.New(title, options.kind, options.priority, repositories, time.Now())
	if err != nil {
		return writeError(stderr, asJSON, err)
	}
	created.Parent = options.parent
	created.BlockedBy = options.blockedBy
	created.Tags = options.tags
	created, err = task.Create(paths.Tasks, created)
	if err != nil {
		return writeError(stderr, asJSON, err)
	}

	return writeResult(stdout, asJSON, created.Summary(), func() {
		fmt.Fprintf(stdout, "Created %s: %s\n", created.ID, created.Title)
	})
}

func runShow(paths ledger.Paths, arguments []string, asJSON bool, stdout, stderr io.Writer) int {
	if len(arguments) != 1 {
		return writeError(stderr, asJSON, errors.New("usage: work-ledger show <task-id>"))
	}
	if _, err := ledger.Load(paths); err != nil {
		return writeError(stderr, asJSON, err)
	}
	tasks, err := task.LoadAll(paths.Tasks, paths.Archive)
	if err != nil {
		return writeError(stderr, asJSON, err)
	}
	selected, err := task.Find(tasks, arguments[0])
	if err != nil {
		return writeError(stderr, asJSON, err)
	}
	return writeResult(stdout, asJSON, selected, func() {
		writeTaskShow(stdout, selected)
	})
}

func writeTaskShow(stdout io.Writer, selected task.Task) {
	fmt.Fprintf(stdout, "%s (%s, %s)\n", selected.Title, selected.Status, selected.ID)
	fmt.Fprintf(stdout, "Kind: %s\n", selected.Kind)
	fmt.Fprintf(stdout, "Priority: %s\n", selected.Priority)
	fmt.Fprintf(stdout, "Repositories: %s\n", formatRepositoryScopes(selected.Repositories))
	if len(selected.Tags) > 0 {
		fmt.Fprintf(stdout, "Tags: %s\n", strings.Join(selected.Tags, ", "))
	}
	if selected.Parent != "" {
		fmt.Fprintf(stdout, "Parent: %s\n", selected.Parent)
	}
	if len(selected.BlockedBy) > 0 {
		fmt.Fprintf(stdout, "Blocked by: %s\n", strings.Join(selected.BlockedBy, ", "))
	}
	if selected.Claim != nil {
		fmt.Fprintf(stdout, "Claim: %s (expires %s)\n", selected.Claim.Agent, selected.Claim.ExpiresAt)
	}
	fmt.Fprintf(stdout, "Created: %s\n", selected.CreatedAt)
	fmt.Fprintf(stdout, "Updated: %s\n\n", selected.UpdatedAt)
	fmt.Fprint(stdout, selected.Body)
}

func formatRepositoryScopes(scopes []task.RepositoryScope) string {
	if len(scopes) == 0 {
		return "none"
	}

	formatted := make([]string, 0, len(scopes))
	for _, scope := range scopes {
		if len(scope.Paths) == 0 {
			formatted = append(formatted, scope.ID)
			continue
		}
		formatted = append(formatted, fmt.Sprintf("%s (paths: %s)", scope.ID, strings.Join(scope.Paths, ", ")))
	}
	return strings.Join(formatted, "; ")
}

func runList(paths ledger.Paths, arguments []string, asJSON bool, stdout, stderr io.Writer) int {
	options, err := parseListArguments(arguments)
	if err != nil {
		return writeError(stderr, asJSON, err)
	}
	config, err := ledger.Load(paths)
	if err != nil {
		return writeError(stderr, asJSON, err)
	}
	if options.here && options.repository != "" {
		return writeError(stderr, asJSON, errors.New("--here and --repo cannot be used together"))
	}
	if options.all && (options.here || options.repository != "") {
		return writeError(stderr, asJSON, errors.New("--all cannot be combined with --here or --repo"))
	}
	if options.here {
		directory, err := os.Getwd()
		if err != nil {
			return writeError(stderr, asJSON, fmt.Errorf("determine current directory: %w", err))
		}
		context, err := repository.Discover(directory)
		if err != nil {
			return writeError(stderr, asJSON, err)
		}
		options.repository = repository.ResolveAlias(context.ID, config.RepositoryAliases)
	} else if options.repository != "" {
		scope, err := resolveRepositoryScope(options.repository, config)
		if err != nil {
			return writeError(stderr, asJSON, err)
		}
		options.repository = scope.ID
	}

	tasks, err := task.LoadAll(paths.Tasks)
	if err != nil {
		return writeError(stderr, asJSON, err)
	}
	matching := task.FilterTasks(tasks, task.Filter{
		Repository: options.repository,
		Parent:     options.parent,
		Statuses:   options.statuses,
		Tags:       options.tags,
		Ready:      options.ready,
	})
	summaries := make([]task.Summary, 0, len(matching))
	for _, matchedTask := range matching {
		summaries = append(summaries, matchedTask.Summary())
	}

	return writeResult(stdout, asJSON, summaries, func() {
		writeTaskList(stdout, summaries)
	})
}

func runSearch(paths ledger.Paths, arguments []string, asJSON bool, stdout, stderr io.Writer) int {
	if len(arguments) == 0 {
		return writeError(stderr, asJSON, errors.New("usage: work-ledger search <text>"))
	}
	if _, err := ledger.Load(paths); err != nil {
		return writeError(stderr, asJSON, err)
	}
	tasks, err := task.LoadAll(paths.Tasks)
	if err != nil {
		return writeError(stderr, asJSON, err)
	}
	matching := task.FilterTasks(tasks, task.Filter{Query: strings.Join(arguments, " ")})
	summaries := make([]task.Summary, 0, len(matching))
	for _, matchedTask := range matching {
		summaries = append(summaries, matchedTask.Summary())
	}
	return writeResult(stdout, asJSON, summaries, func() {
		writeTaskList(stdout, summaries)
	})
}

func parseNewArguments(arguments []string) (newOptions, string, error) {
	var options newOptions
	var titles []string
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		switch argument {
		case "--repo", "--tag", "--blocked-by", "--kind", "--priority", "--parent":
			if index+1 >= len(arguments) {
				return options, "", fmt.Errorf("%s requires a value", argument)
			}
			index++
			value := arguments[index]
			switch argument {
			case "--repo":
				options.repositories = append(options.repositories, value)
			case "--tag":
				options.tags = append(options.tags, value)
			case "--blocked-by":
				options.blockedBy = append(options.blockedBy, value)
			case "--kind":
				options.kind = value
			case "--priority":
				options.priority = value
			case "--parent":
				options.parent = value
			}
		default:
			if strings.HasPrefix(argument, "-") {
				return options, "", fmt.Errorf("unknown new option %q", argument)
			}
			titles = append(titles, argument)
		}
	}
	if len(titles) != 1 {
		return options, "", errors.New("usage: work-ledger new <title> [--repo <path-or-id>]...")
	}
	return options, titles[0], nil
}

func parseListArguments(arguments []string) (listOptions, error) {
	var options listOptions
	for index := 0; index < len(arguments); index++ {
		argument := arguments[index]
		switch argument {
		case "--all":
			options.all = true
		case "--here":
			options.here = true
		case "--ready":
			options.ready = true
		case "--repo", "--parent", "--status", "--tag":
			if index+1 >= len(arguments) {
				return options, fmt.Errorf("%s requires a value", argument)
			}
			index++
			value := arguments[index]
			switch argument {
			case "--repo":
				if options.repository != "" {
					return options, errors.New("--repo can only be specified once")
				}
				options.repository = value
			case "--parent":
				if options.parent != "" {
					return options, errors.New("--parent can only be specified once")
				}
				options.parent = value
			case "--status":
				options.statuses = append(options.statuses, value)
			case "--tag":
				options.tags = append(options.tags, value)
			}
		default:
			return options, fmt.Errorf("unknown list option %q", argument)
		}
	}
	return options, nil
}

func resolveRepositoryScopes(values []string, config ledger.Config) ([]task.RepositoryScope, error) {
	scopes := make([]task.RepositoryScope, 0, len(values))
	for _, value := range values {
		scope, err := resolveRepositoryScope(value, config)
		if err != nil {
			return nil, err
		}
		scopes = append(scopes, scope)
	}
	return scopes, nil
}

func resolveRepositoryScope(value string, config ledger.Config) (task.RepositoryScope, error) {
	if strings.TrimSpace(value) == "" {
		return task.RepositoryScope{}, errors.New("repository must not be empty")
	}
	if info, err := os.Stat(value); err == nil && info.IsDir() {
		context, err := repository.Discover(value)
		if err != nil {
			return task.RepositoryScope{}, err
		}
		return task.RepositoryScope{ID: repository.ResolveAlias(context.ID, config.RepositoryAliases)}, nil
	}
	if filepath.IsAbs(value) || strings.HasPrefix(value, ".") {
		return task.RepositoryScope{}, fmt.Errorf("repository path %q does not exist or is not a directory", value)
	}
	return task.RepositoryScope{ID: repository.ResolveAlias(value, config.RepositoryAliases)}, nil
}

func writeTaskList(stdout io.Writer, tasks []task.Summary) {
	if len(tasks) == 0 {
		fmt.Fprintln(stdout, "No tasks found.")
		return
	}
	for _, task := range tasks {
		fmt.Fprintf(stdout, "%s  %-9s %-8s %s\n", task.ID, task.Status, task.Priority, task.Title)
	}
}
