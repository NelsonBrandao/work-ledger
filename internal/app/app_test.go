package app

import (
	"bytes"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"

	"github.com/NelsonBrandao/work-ledger/internal/task"
)

func TestInitAndDoctorJSON(t *testing.T) {
	t.Parallel()

	home := filepath.Join(t.TempDir(), "ledger")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if exit := Run([]string{"--home", home, "--json", "init"}, &stdout, &stderr); exit != 0 {
		t.Fatalf("init exit = %d, stderr = %s", exit, stderr.String())
	}

	var result map[string]any
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode init result: %v", err)
	}
	if result["created"] != true {
		t.Errorf("created = %v, want true", result["created"])
	}

	stdout.Reset()
	stderr.Reset()
	if exit := Run([]string{"--home", home, "--json", "doctor"}, &stdout, &stderr); exit != 0 {
		t.Fatalf("doctor exit = %d, stderr = %s", exit, stderr.String())
	}
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode doctor result: %v", err)
	}
	if result["task_count"] != float64(0) {
		t.Errorf("task_count = %v, want 0", result["task_count"])
	}
}

func TestUnknownCommand(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if exit := Run([]string{"unknown"}, &stdout, &stderr); exit != 1 {
		t.Fatalf("Run exit = %d, want 1", exit)
	}
	if stderr.Len() == 0 {
		t.Fatal("Run did not explain the unknown command")
	}
}

func TestCommandSpecificHelp(t *testing.T) {
	t.Parallel()

	for _, arguments := range [][]string{
		{"update", "--help"},
		{"help", "update"},
	} {
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		if exit := Run(arguments, &stdout, &stderr); exit != 0 {
			t.Fatalf("%v exit = %d, stderr = %s", arguments, exit, stderr.String())
		}
		for _, fragment := range []string{
			"Usage: work-ledger update <task-id>",
			"Supplying any --repo, --tag, or --blocked-by flag replaces that complete list",
			"Use the etag",
		} {
			if !strings.Contains(stdout.String(), fragment) {
				t.Fatalf("%v help does not contain %q:\n%s", arguments, fragment, stdout.String())
			}
		}
	}
}

func TestEveryCommandHasHelp(t *testing.T) {
	t.Parallel()

	for _, command := range []string{
		"init", "doctor", "context", "new", "show", "list", "search", "claim",
		"release", "note", "update", "split", "close", "archive", "delete", "import", "prime",
	} {
		var stdout bytes.Buffer
		var stderr bytes.Buffer
		if exit := Run([]string{command, "--help"}, &stdout, &stderr); exit != 0 {
			t.Fatalf("%s --help exit = %d, stderr = %s", command, exit, stderr.String())
		}
		if !strings.Contains(stdout.String(), "Usage: work-ledger "+command) {
			t.Fatalf("%s --help output = %s", command, stdout.String())
		}
	}
}

func TestHelpHelpShowsGeneralUsage(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if exit := Run([]string{"help", "--help"}, &stdout, &stderr); exit != 0 {
		t.Fatalf("help --help exit = %d, stderr = %s", exit, stderr.String())
	}
	if !strings.Contains(stdout.String(), "Commands:") {
		t.Fatalf("help --help output = %s", stdout.String())
	}
}

func TestCommandSpecificHelpJSON(t *testing.T) {
	t.Parallel()

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if exit := Run([]string{"--json", "update", "--help"}, &stdout, &stderr); exit != 0 {
		t.Fatalf("update help exit = %d, stderr = %s", exit, stderr.String())
	}
	var result helpResult
	if err := json.Unmarshal(stdout.Bytes(), &result); err != nil {
		t.Fatalf("decode update help: %v", err)
	}
	if result.Command != "update" || !strings.Contains(result.Usage, "--if-match ETAG") || len(result.Notes) < 2 {
		t.Fatalf("help result = %#v", result)
	}
}

func TestNewShowListAndSearchJSON(t *testing.T) {
	t.Parallel()

	home := filepath.Join(t.TempDir(), "ledger")
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if exit := Run([]string{"--home", home, "init"}, &stdout, &stderr); exit != 0 {
		t.Fatalf("init exit = %d, stderr = %s", exit, stderr.String())
	}

	stdout.Reset()
	stderr.Reset()
	if exit := Run([]string{
		"--home", home,
		"new", "Migrate publish API",
		"--repo", "github.com/customerio/parcel",
		"--repo", "github.com/customerio/services",
		"--tag", "migration",
		"--json",
	}, &stdout, &stderr); exit != 0 {
		t.Fatalf("new exit = %d, stderr = %s", exit, stderr.String())
	}

	var created task.Summary
	if err := json.Unmarshal(stdout.Bytes(), &created); err != nil {
		t.Fatalf("decode new result: %v", err)
	}
	if len(created.Repositories) != 2 {
		t.Errorf("repositories = %#v", created.Repositories)
	}

	stdout.Reset()
	stderr.Reset()
	if exit := Run([]string{"--home", home, "show", created.ID, "--json"}, &stdout, &stderr); exit != 0 {
		t.Fatalf("show exit = %d, stderr = %s", exit, stderr.String())
	}
	var shown task.Task
	if err := json.Unmarshal(stdout.Bytes(), &shown); err != nil {
		t.Fatalf("decode show result: %v", err)
	}
	if shown.Title != created.Title || shown.Body == "" {
		t.Errorf("shown task = %#v", shown)
	}

	for _, command := range [][]string{
		{"--home", home, "list", "--repo", "github.com/customerio/services", "--json"},
		{"--home", home, "list", "--ready", "--json"},
		{"--home", home, "search", "publish", "--json"},
	} {
		stdout.Reset()
		stderr.Reset()
		if exit := Run(command, &stdout, &stderr); exit != 0 {
			t.Fatalf("%v exit = %d, stderr = %s", command, exit, stderr.String())
		}
		var listed []task.Summary
		if err := json.Unmarshal(stdout.Bytes(), &listed); err != nil {
			t.Fatalf("decode %v result: %v", command, err)
		}
		if len(listed) != 1 || listed[0].ID != created.ID {
			t.Errorf("%v listed %#v", command, listed)
		}
	}
}

func TestShowHumanOutputIncludesTaskMetadata(t *testing.T) {
	t.Parallel()

	home := filepath.Join(t.TempDir(), "ledger")
	runCommand(t, nil, "--home", home, "init")

	parentOutput := runCommand(t, nil, "--home", home, "new", "Parent", "--json")
	var parent task.Summary
	if err := json.Unmarshal(parentOutput, &parent); err != nil {
		t.Fatalf("decode parent: %v", err)
	}
	blockerOutput := runCommand(t, nil, "--home", home, "new", "Blocker", "--json")
	var blocker task.Summary
	if err := json.Unmarshal(blockerOutput, &blocker); err != nil {
		t.Fatalf("decode blocker: %v", err)
	}

	createdOutput := runCommand(t, nil,
		"--home", home,
		"new", "Visible metadata",
		"--kind", "feature",
		"--priority", "high",
		"--repo", "github.com/customerio/parcel",
		"--tag", "migration",
		"--parent", parent.ID,
		"--blocked-by", blocker.ID,
		"--json",
	)
	var created task.Summary
	if err := json.Unmarshal(createdOutput, &created); err != nil {
		t.Fatalf("decode task: %v", err)
	}
	runCommand(t, nil, "--home", home, "claim", created.ID, "--agent", "metadata-test")

	shown := string(runCommand(t, nil, "--home", home, "show", created.ID))
	for _, fragment := range []string{
		"Visible metadata (active, " + created.ID + ")",
		"Kind: feature",
		"Priority: high",
		"Repositories: github.com/customerio/parcel",
		"Tags: migration",
		"Parent: " + parent.ID,
		"Blocked by: " + blocker.ID,
		"Claim: metadata-test (expires ",
		"Created: ",
		"Updated: ",
		"# Visible metadata",
	} {
		if !strings.Contains(shown, fragment) {
			t.Fatalf("show output does not contain %q:\n%s", fragment, shown)
		}
	}
}

func TestLifecycleCommands(t *testing.T) {
	t.Parallel()

	home := filepath.Join(t.TempDir(), "ledger")
	runCommand(t, nil, "--home", home, "init")
	createdOutput := runCommand(t, nil,
		"--home", home,
		"new", "Lifecycle parent",
		"--repo", "github.com/customerio/parcel",
		"--json",
	)
	var parent task.Summary
	if err := json.Unmarshal(createdOutput, &parent); err != nil {
		t.Fatal(err)
	}

	shownOutput := runCommand(t, nil, "--home", home, "show", parent.ID, "--json")
	var shown task.Task
	if err := json.Unmarshal(shownOutput, &shown); err != nil {
		t.Fatal(err)
	}
	runCommand(t, nil, "--home", home, "claim", parent.ID, "--agent", "test-agent", "--json")
	runCommand(t, strings.NewReader("A note from stdin"), "--home", home, "note", parent.ID, "--agent", "test-agent", "--json")

	shownOutput = runCommand(t, nil, "--home", home, "show", parent.ID, "--json")
	if err := json.Unmarshal(shownOutput, &shown); err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(shown.Body, "A note from stdin") {
		t.Fatalf("note was not stored: %q", shown.Body)
	}
	runCommand(t, nil, "--home", home, "update", parent.ID, "--priority", "high", "--if-match", shown.ETag, "--agent", "test-agent", "--json")

	childOutput := runCommand(t, nil, "--home", home, "split", parent.ID, "Lifecycle child", "--json")
	var child task.Summary
	if err := json.Unmarshal(childOutput, &child); err != nil {
		t.Fatal(err)
	}
	runCommand(t, nil, "--home", home, "claim", child.ID, "--agent", "test-agent", "--json")
	runCommand(t, nil, "--home", home, "release", child.ID, "--agent", "test-agent", "--json")
	listOutput := runCommand(t, nil, "--home", home, "list", "--parent", parent.ID, "--json")
	var children []task.Summary
	if err := json.Unmarshal(listOutput, &children); err != nil {
		t.Fatal(err)
	}
	if len(children) != 1 || children[0].ID != child.ID {
		t.Fatalf("children = %#v", children)
	}

	runCommand(t, nil, "--home", home, "close", parent.ID, "Lifecycle verified", "--agent", "test-agent", "--json")
	runCommand(t, nil, "--home", home, "archive", "--yes", "--json")
	runCommand(t, nil, "--home", home, "delete", child.ID, "--yes", "--json")
	prime := runCommand(t, nil, "--home", home, "prime", "--json")
	var result primeResult
	if err := json.Unmarshal(prime, &result); err != nil {
		t.Fatalf("decode prime output: %v", err)
	}
	if len(result.Protocol) < 5 || len(result.Commands) < 10 || len(result.Values["status"]) != 5 {
		t.Fatalf("prime result = %#v", result)
	}
	if !strings.Contains(result.Commands[0].Syntax, "--home PATH") {
		t.Fatalf("prime commands = %#v", result.Commands)
	}
}

func TestPrimeHumanOutputIsSelfContained(t *testing.T) {
	t.Parallel()

	home := filepath.Join(t.TempDir(), "ledger")
	runCommand(t, nil, "--home", home, "init")
	prime := runCommand(t, nil, "--home", home, "prime")
	for _, fragment := range []string{
		"# Work Ledger Agent Guide",
		"## Required protocol",
		"work-ledger new <title>",
		"work-ledger update <task-id>",
		"work-ledger import beans <directory>",
		"priority: critical, high, normal, low",
	} {
		if !bytes.Contains(prime, []byte(fragment)) {
			t.Fatalf("prime does not contain %q:\n%s", fragment, prime)
		}
	}
}

func TestNoteRejectsForce(t *testing.T) {
	t.Parallel()

	home := filepath.Join(t.TempDir(), "ledger")
	runCommand(t, nil, "--home", home, "init")
	createdOutput := runCommand(t, nil, "--home", home, "new", "No force note", "--json")
	var created task.Summary
	if err := json.Unmarshal(createdOutput, &created); err != nil {
		t.Fatal(err)
	}

	var stdout bytes.Buffer
	var stderr bytes.Buffer
	if exit := Run([]string{"--home", home, "note", created.ID, "--force"}, &stdout, &stderr); exit != 1 {
		t.Fatalf("note --force exit = %d, want 1", exit)
	}
	if !strings.Contains(stderr.String(), "unknown note option") {
		t.Fatalf("note --force error = %q", stderr.String())
	}
}

func runCommand(t *testing.T, stdin *strings.Reader, arguments ...string) []byte {
	t.Helper()
	var stdout bytes.Buffer
	var stderr bytes.Buffer
	input := strings.NewReader("")
	if stdin != nil {
		input = stdin
	}
	if exit := RunWithInput(arguments, input, &stdout, &stderr); exit != 0 {
		t.Fatalf("%v exit = %d, stderr = %s", arguments, exit, stderr.String())
	}
	return stdout.Bytes()
}
