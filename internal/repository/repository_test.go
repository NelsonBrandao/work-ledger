package repository

import (
	"os/exec"
	"path/filepath"
	"testing"
)

func TestNormalizeRemote(t *testing.T) {
	t.Parallel()

	tests := map[string]string{
		"git@github.com:NelsonBrandao/work-ledger.git":     "github.com/NelsonBrandao/work-ledger",
		"ssh://git@github.com/NelsonBrandao/work-ledger":   "github.com/NelsonBrandao/work-ledger",
		"https://github.com/NelsonBrandao/work-ledger.git": "github.com/NelsonBrandao/work-ledger",
	}

	for remote, want := range tests {
		remote, want := remote, want
		t.Run(remote, func(t *testing.T) {
			t.Parallel()
			got, err := NormalizeRemote(remote)
			if err != nil {
				t.Fatalf("NormalizeRemote returned an error: %v", err)
			}
			if got != want {
				t.Errorf("NormalizeRemote(%q) = %q, want %q", remote, got, want)
			}
		})
	}
}

func TestResolveAlias(t *testing.T) {
	t.Parallel()

	aliases := map[string][]string{
		"github.com/customerio/parcel": {"github.com/NelsonBrandao/parcel"},
	}
	if got := ResolveAlias("github.com/NelsonBrandao/parcel", aliases); got != "github.com/customerio/parcel" {
		t.Errorf("ResolveAlias = %q", got)
	}
}

func TestDiscover(t *testing.T) {
	directory := t.TempDir()
	runGitCommand(t, directory, "init")
	runGitCommand(t, directory, "remote", "add", "origin", "git@github.com:NelsonBrandao/work-ledger.git")
	statusBefore := gitOutput(t, directory, "status", "--porcelain")

	context, err := Discover(directory)
	if err != nil {
		t.Fatalf("Discover returned an error: %v", err)
	}
	expectedRoot, err := filepath.EvalSymlinks(directory)
	if err != nil {
		t.Fatalf("resolve temporary repository path: %v", err)
	}
	if context.Root != expectedRoot {
		t.Errorf("root = %q, want %q", context.Root, expectedRoot)
	}
	if context.ID != "github.com/NelsonBrandao/work-ledger" {
		t.Errorf("ID = %q", context.ID)
	}
	if statusAfter := gitOutput(t, directory, "status", "--porcelain"); statusAfter != statusBefore {
		t.Errorf("Discover changed Git status from %q to %q", statusBefore, statusAfter)
	}
}

func runGitCommand(t *testing.T, directory string, arguments ...string) {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", directory}, arguments...)...)
	if output, err := command.CombinedOutput(); err != nil {
		t.Fatalf("git %v: %s", arguments, output)
	}
}

func gitOutput(t *testing.T, directory string, arguments ...string) string {
	t.Helper()
	command := exec.Command("git", append([]string{"-C", directory}, arguments...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("git %v: %s", arguments, output)
	}
	return string(output)
}
