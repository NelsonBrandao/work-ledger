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
	if !bytes.Contains(prime, []byte("instructions")) {
		t.Fatalf("prime output = %s", prime)
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
