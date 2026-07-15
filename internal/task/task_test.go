package task

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const validTask = `---
schema: work-ledger/task/v1
id: wl-01J
title: Test task
kind: task
status: todo
priority: normal
repositories:
  - id: github.com/customerio/parcel
created_at: 2026-07-15T13:00:00Z
updated_at: 2026-07-15T13:00:00Z
---

# Test task
`

func TestParse(t *testing.T) {
	t.Parallel()

	task, err := Parse(validTask)
	if err != nil {
		t.Fatalf("Parse returned an error: %v", err)
	}
	if task.ID != "wl-01J" {
		t.Errorf("ID = %q, want wl-01J", task.ID)
	}
	if task.Repositories[0].ID != "github.com/customerio/parcel" {
		t.Errorf("repository = %q", task.Repositories[0].ID)
	}
}

func TestParseRejectsInvalidStatus(t *testing.T) {
	t.Parallel()

	if _, err := Parse(strings.Replace(validTask, "status: todo", "status: unknown", 1)); err == nil {
		t.Fatal("Parse succeeded for an invalid status")
	}
}

func TestParseRequiresFrontmatter(t *testing.T) {
	t.Parallel()

	if _, err := Parse("# no frontmatter\n"); err == nil {
		t.Fatal("Parse succeeded without frontmatter")
	}
}

func TestNewCreateAndLoad(t *testing.T) {
	t.Parallel()

	created, err := New("Plan Parcel & Services", "", "", []RepositoryScope{
		{ID: "github.com/customerio/parcel"},
		{ID: "github.com/customerio/services"},
	}, time.Date(2026, time.July, 15, 14, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("New returned an error: %v", err)
	}
	if !strings.HasPrefix(created.ID, "wl-") {
		t.Errorf("ID = %q, want a wl- prefix", created.ID)
	}
	if got := FileName(created); !strings.HasSuffix(got, "-plan-parcel-services.md") {
		t.Errorf("FileName = %q", got)
	}

	directory := t.TempDir()
	created, err = Create(directory, created)
	if err != nil {
		t.Fatalf("Create returned an error: %v", err)
	}
	info, err := os.Stat(created.Path)
	if err != nil {
		t.Fatalf("stat created task: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("created task permissions = %v, want 0600", info.Mode().Perm())
	}

	loaded, err := Load(filepath.Join(directory, FileName(created)))
	if err != nil {
		t.Fatalf("Load returned an error: %v", err)
	}
	if loaded.Title != created.Title {
		t.Errorf("loaded title = %q, want %q", loaded.Title, created.Title)
	}
	if !strings.Contains(loaded.Body, "## Investigation") {
		t.Errorf("created body = %q, want an Investigation section", loaded.Body)
	}
}

func TestFilterTasksReadyAndSearch(t *testing.T) {
	t.Parallel()

	tasks := []Task{
		{ID: "wl-done", Title: "Completed blocker", Status: "done", Kind: "task", Priority: "normal"},
		{ID: "wl-ready", Title: "Fix parcel editor", Status: "todo", Kind: "bug", Priority: "high", Tags: []string{"editor"}, BlockedBy: []string{"wl-done"}, Repositories: []RepositoryScope{{ID: "github.com/customerio/parcel"}}},
		{ID: "wl-blocked", Title: "Wait for API", Status: "todo", Kind: "task", Priority: "normal", BlockedBy: []string{"wl-missing"}},
	}

	ready := FilterTasks(tasks, Filter{Ready: true})
	if len(ready) != 1 || ready[0].ID != "wl-ready" {
		t.Errorf("ready tasks = %#v", ready)
	}
	matching := FilterTasks(tasks, Filter{Repository: "github.com/customerio/parcel", Query: "editor", Tags: []string{"editor"}})
	if len(matching) != 1 || matching[0].ID != "wl-ready" {
		t.Errorf("matching tasks = %#v", matching)
	}
}
