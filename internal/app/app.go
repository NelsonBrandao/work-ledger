package app

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/NelsonBrandao/work-ledger/internal/ledger"
	"github.com/NelsonBrandao/work-ledger/internal/repository"
	"github.com/NelsonBrandao/work-ledger/internal/task"
)

const usage = `Usage: work-ledger [--home PATH] [--json] <command>

Commands:
  init       Create or validate a private ledger directory.
  doctor     Validate ledger configuration and task files.
  context    Show the current Git repository identity and ledger location.
  new        Create a Markdown task.
  show       Render one task, including its plan and investigation.
  list       List tasks, optionally for the current repository or ready to start.
  search     Search task metadata and Markdown bodies.
  claim      Claim a task for a time-bound agent session.
  release    Release the current agent's claim.
  note       Append a timestamped Activity note.
  update     Update explicit task metadata with conflict protection.
  split      Create a child task.
  close      Mark a task done with a summary.
  archive    Move completed tasks to the archive.
  delete     Permanently remove a task with --yes.
  import     Import tasks from a Beans directory.
  prime      Print a complete operating guide for coding agents.
`

type options struct {
	home string
	json bool
}

func Run(arguments []string, stdout, stderr io.Writer) int {
	return RunWithInput(arguments, os.Stdin, stdout, stderr)
}

func RunWithInput(arguments []string, stdin io.Reader, stdout, stderr io.Writer) int {
	options, command, commandArguments, err := parseArguments(arguments)
	if err != nil {
		return writeError(stderr, options.json, err)
	}
	if command == "" || command == "help" || command == "--help" || command == "-h" {
		_, _ = io.WriteString(stdout, usage)
		return 0
	}

	home, err := resolveHome(options.home)
	if err != nil {
		return writeError(stderr, options.json, err)
	}
	paths := ledger.NewPaths(home)

	switch command {
	case "init":
		if len(commandArguments) > 0 {
			return writeError(stderr, options.json, fmt.Errorf("init does not accept arguments"))
		}
		return runInit(paths, options.json, stdout, stderr)
	case "doctor":
		if len(commandArguments) > 0 {
			return writeError(stderr, options.json, fmt.Errorf("doctor does not accept arguments"))
		}
		return runDoctor(paths, options.json, stdout, stderr)
	case "context":
		if len(commandArguments) > 0 {
			return writeError(stderr, options.json, fmt.Errorf("context does not accept arguments"))
		}
		return runContext(paths, options.json, stdout, stderr)
	case "new":
		return runNew(paths, commandArguments, options.json, stdout, stderr)
	case "show":
		return runShow(paths, commandArguments, options.json, stdout, stderr)
	case "list":
		return runList(paths, commandArguments, options.json, stdout, stderr)
	case "search":
		return runSearch(paths, commandArguments, options.json, stdout, stderr)
	case "claim":
		return runClaim(paths, commandArguments, options.json, stdout, stderr)
	case "release":
		return runRelease(paths, commandArguments, options.json, stdout, stderr)
	case "note":
		return runNote(paths, commandArguments, stdin, options.json, stdout, stderr)
	case "update":
		return runUpdate(paths, commandArguments, options.json, stdout, stderr)
	case "split":
		return runSplit(paths, commandArguments, options.json, stdout, stderr)
	case "close":
		return runClose(paths, commandArguments, options.json, stdout, stderr)
	case "archive":
		return runArchive(paths, commandArguments, options.json, stdout, stderr)
	case "delete":
		return runDelete(paths, commandArguments, options.json, stdout, stderr)
	case "import":
		return runImport(paths, commandArguments, options.json, stdout, stderr)
	case "prime":
		return runPrime(paths, commandArguments, options.json, stdout, stderr)
	default:
		return writeError(stderr, options.json, fmt.Errorf("unknown command %q", command))
	}
}

func parseArguments(arguments []string) (options, string, []string, error) {
	var options options
	var remaining []string
	for index := 0; index < len(arguments); index++ {
		switch arguments[index] {
		case "--json":
			options.json = true
		case "--home":
			if index+1 >= len(arguments) {
				return options, "", nil, errors.New("--home requires a path")
			}
			index++
			options.home = arguments[index]
		default:
			remaining = append(remaining, arguments[index])
		}
	}
	if len(remaining) == 0 {
		return options, "", nil, nil
	}
	return options, remaining[0], remaining[1:], nil
}

func resolveHome(flagValue string) (string, error) {
	userHome, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("determine user home: %w", err)
	}
	configuredHome := flagValue
	if configuredHome == "" {
		configuredHome = os.Getenv("WORK_LEDGER_HOME")
	}
	return ledger.DefaultHome(userHome, configuredHome)
}

func runInit(paths ledger.Paths, asJSON bool, stdout, stderr io.Writer) int {
	created, err := ledger.Initialize(paths)
	if err != nil {
		return writeError(stderr, asJSON, err)
	}
	return writeResult(stdout, asJSON, map[string]any{"home": paths.Home, "created": created}, func() {
		if created {
			fmt.Fprintf(stdout, "Initialized ledger at %s\n", paths.Home)
			return
		}
		fmt.Fprintf(stdout, "Ledger already exists at %s\n", paths.Home)
	})
}

func runDoctor(paths ledger.Paths, asJSON bool, stdout, stderr io.Writer) int {
	if _, err := ledger.Load(paths); err != nil {
		return writeError(stderr, asJSON, err)
	}
	tasks, err := task.LoadAll(paths.Tasks, paths.Archive)
	if err != nil {
		return writeError(stderr, asJSON, err)
	}
	if err := task.ValidateCollection(tasks); err != nil {
		return writeError(stderr, asJSON, err)
	}
	return writeResult(stdout, asJSON, map[string]any{"home": paths.Home, "task_count": len(tasks), "healthy": true}, func() {
		fmt.Fprintf(stdout, "Ledger is healthy: %d task(s) at %s\n", len(tasks), paths.Home)
	})
}

func runContext(paths ledger.Paths, asJSON bool, stdout, stderr io.Writer) int {
	directory, err := os.Getwd()
	if err != nil {
		return writeError(stderr, asJSON, fmt.Errorf("determine current directory: %w", err))
	}
	context, err := repository.Discover(directory)
	if err != nil {
		return writeError(stderr, asJSON, err)
	}

	initialized := true
	config, err := ledger.Load(paths)
	if err != nil {
		if !errors.Is(err, os.ErrNotExist) {
			return writeError(stderr, asJSON, err)
		}
		initialized = false
	} else {
		context.ID = repository.ResolveAlias(context.ID, config.RepositoryAliases)
	}

	return writeResult(stdout, asJSON, map[string]any{
		"ledger_home": paths.Home,
		"initialized": initialized,
		"repository":  context,
	}, func() {
		fmt.Fprintf(stdout, "Ledger: %s\nInitialized: %t\nRepository: %s\nRoot: %s\nRemote: %s\n", paths.Home, initialized, context.ID, context.Root, context.Remote)
	})
}

func writeResult(stdout io.Writer, asJSON bool, value any, human func()) int {
	if !asJSON {
		human()
		return 0
	}
	encoder := json.NewEncoder(stdout)
	if err := encoder.Encode(value); err != nil {
		return 1
	}
	return 0
}

func writeError(stderr io.Writer, asJSON bool, err error) int {
	if asJSON {
		_ = json.NewEncoder(stderr).Encode(map[string]string{"error": err.Error()})
		return 1
	}
	fmt.Fprintf(stderr, "work-ledger: %v\n", err)
	return 1
}
