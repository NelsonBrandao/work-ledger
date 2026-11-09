package store

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/NelsonBrandao/work-ledger/internal/ledger"
	"github.com/NelsonBrandao/work-ledger/internal/task"
)

func TestMutateUsesETagAndPreservesUnknownFrontmatter(t *testing.T) {
	paths := newLedger(t)
	created := newTask(t, paths, "Update me")
	contents, err := os.ReadFile(created.Path)
	if err != nil {
		t.Fatal(err)
	}
	contents = []byte(strings.Replace(string(contents), "---\n\n# Update me", "custom_field: keep-me\n---\n\n# Update me", 1))
	if err := os.WriteFile(created.Path, contents, 0o600); err != nil {
		t.Fatal(err)
	}
	// Reload after the manual frontmatter edit so the mutation uses its ETag.
	document, err := task.LoadDocument(created.Path)
	if err != nil {
		t.Fatal(err)
	}

	updated, err := Mutate(paths, created.ID, document.ETag, func(document *task.Document) error {
		document.Priority = "high"
		return nil
	})
	if err != nil {
		t.Fatalf("Mutate returned an error: %v", err)
	}
	if updated.Priority != "high" || updated.ETag == document.ETag {
		t.Errorf("updated task = %#v", updated)
	}
	persisted, err := os.ReadFile(created.Path)
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(persisted), "custom_field: keep-me") {
		t.Fatal("mutation did not preserve custom frontmatter")
	}

	if _, err := Mutate(paths, created.ID, document.ETag, func(document *task.Document) error { return nil }); !errors.Is(err, ErrConflict) {
		t.Fatalf("stale mutation error = %v, want ErrConflict", err)
	}
}

func TestArchiveAndDelete(t *testing.T) {
	paths := newLedger(t)
	completed := newTask(t, paths, "Archive me")
	if _, err := Mutate(paths, completed.ID, "", func(document *task.Document) error {
		document.Status = "done"
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	archived, err := Archive(paths)
	if err != nil {
		t.Fatal(err)
	}
	if len(archived) != 1 {
		t.Fatalf("archived = %#v", archived)
	}

	deletable := newTask(t, paths, "Delete me")
	if _, err := Delete(paths, deletable.ID); err != nil {
		t.Fatal(err)
	}
}

func TestDeleteRejectsReferencedTask(t *testing.T) {
	paths := newLedger(t)
	parent := newTask(t, paths, "Parent")
	child := newTask(t, paths, "Child")
	if _, err := Mutate(paths, child.ID, "", func(document *task.Document) error {
		document.Parent = parent.ID
		return nil
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := Delete(paths, parent.ID); err == nil {
		t.Fatal("Delete succeeded for a referenced task")
	}
}

func newLedger(t *testing.T) ledger.Paths {
	t.Helper()
	paths := ledger.NewPaths(filepath.Join(t.TempDir(), "ledger"))
	if _, err := ledger.Initialize(paths); err != nil {
		t.Fatal(err)
	}
	return paths
}

func newTask(t *testing.T, paths ledger.Paths, title string) task.Task {
	t.Helper()
	created, err := task.New(title, "task", "normal", nil, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	created, err = task.Create(paths.Tasks, created, nil)
	if err != nil {
		t.Fatal(err)
	}
	return created
}

func TestDeleteRequiresTheFullID(t *testing.T) {
	t.Parallel()

	paths := newLedger(t)
	created := newTask(t, paths, "Remove me")
	if _, err := Delete(paths, "remove"); err == nil || !strings.Contains(err.Error(), "full task ID") {
		t.Fatalf("Delete by prefix error = %v", err)
	}
	if _, err := Delete(paths, created.ID); err != nil {
		t.Fatalf("Delete by ID: %v", err)
	}
}
