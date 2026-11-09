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
  - id: github.com/acme/web
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
	if task.Repositories[0].ID != "github.com/acme/web" {
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

	created, err := New("Plan Web & API", "", "", []RepositoryScope{
		{ID: "github.com/acme/web"},
		{ID: "github.com/acme/api"},
	}, time.Date(2026, time.July, 15, 14, 0, 0, 0, time.UTC))
	if err != nil {
		t.Fatalf("New returned an error: %v", err)
	}
	if created.ID != "plan-web-api" {
		t.Errorf("ID = %q, want plan-web-api", created.ID)
	}
	if got := FileName(created); got != "plan-web-api.md" {
		t.Errorf("FileName = %q", got)
	}

	directory := t.TempDir()
	created, err = Create(directory, created, nil)
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
	if strings.TrimSpace(loaded.Body) != "# Plan Web & API" {
		t.Errorf("created body = %q, want only the title heading", loaded.Body)
	}
}

func TestSlugID(t *testing.T) {
	t.Parallel()

	for _, testCase := range []struct{ title, want string }{
		{"CON-8538: revive Design Studio render warmer (stage 3/9)", "con-8538-revive-design-studio-render"},
		{"Fix: UpdateSnippet ignores URL :id", "fix-updatesnippet-ignores-url-id"},
		{"Café ümlaut", "caf-mlaut"},
		{"Supercalifragilisticexpialidocious-and-then-some-more-words-still", "supercalifragilisticexpialidocious-and-then-some"},
		{"!!!", "task"},
	} {
		if got := slugID(testCase.title); got != testCase.want {
			t.Errorf("slugID(%q) = %q, want %q", testCase.title, got, testCase.want)
		}
	}
}

func TestCreateAddsSuffixWhenIDIsTaken(t *testing.T) {
	t.Parallel()

	directory := t.TempDir()
	now := time.Date(2026, time.July, 15, 14, 0, 0, 0, time.UTC)
	taken := map[string]bool{"archived-one": true}
	var ids []string
	for _, title := range []string{"Same title", "Same title", "Archived one"} {
		created, err := New(title, "", "", nil, now)
		if err != nil {
			t.Fatal(err)
		}
		created, err = Create(directory, created, taken)
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, created.ID)
	}
	if ids[0] != "same-title" {
		t.Errorf("first ID = %q, want same-title", ids[0])
	}
	for _, id := range ids[1:] {
		if len(id) < 5 || id[len(id)-5] != '-' {
			t.Errorf("ID %q has no suffix", id)
		}
	}
	if !strings.HasPrefix(ids[1], "same-title-") || !strings.HasPrefix(ids[2], "archived-one-") {
		t.Errorf("IDs = %v", ids)
	}
}

func TestFindAcceptsUniquePrefix(t *testing.T) {
	t.Parallel()

	tasks := []Task{
		{ID: "fix-login-redirect"},
		{ID: "fix-login-timeout"},
		{ID: "wl-01kp6s06y88hjrre72pfw9chn6", LegacyIDs: []string{"proj-4r6q"}},
	}
	for _, testCase := range []struct{ identifier, want, wantErr string }{
		{identifier: "fix-login-t", want: "fix-login-timeout"},
		{identifier: "fix-login-redirect", want: "fix-login-redirect"},
		{identifier: "proj-4r6q", want: "wl-01kp6s06y88hjrre72pfw9chn6"},
		{identifier: "wl-01kp6", want: "wl-01kp6s06y88hjrre72pfw9chn6"},
		{identifier: "fix-login", wantErr: "ambiguous"},
		{identifier: "missing", wantErr: "not found"},
	} {
		found, err := Find(tasks, testCase.identifier)
		if testCase.wantErr != "" {
			if err == nil || !strings.Contains(err.Error(), testCase.wantErr) {
				t.Errorf("Find(%q) error = %v, want %q", testCase.identifier, err, testCase.wantErr)
			}
			continue
		}
		if err != nil || found.ID != testCase.want {
			t.Errorf("Find(%q) = %q, %v; want %q", testCase.identifier, found.ID, err, testCase.want)
		}
	}
}

func TestStaleClaims(t *testing.T) {
	t.Parallel()

	now := time.Date(2026, time.November, 3, 12, 0, 0, 0, time.UTC)
	tasks := []Task{
		{ID: "held", Status: "active", Claim: &Claim{Agent: "a", ExpiresAt: "2026-11-03T13:00:00Z"}},
		{ID: "expired", Status: "active", Claim: &Claim{Agent: "a", ExpiresAt: "2026-11-03T11:00:00Z"}},
		{ID: "unclaimed", Status: "active"},
		{ID: "todo", Status: "todo"},
	}
	var ready []string
	for _, found := range FilterTasks(tasks, Filter{Ready: true, Now: now}) {
		ready = append(ready, found.ID)
	}
	if strings.Join(ready, ",") != "expired,unclaimed,todo" {
		t.Errorf("ready = %v", ready)
	}
}

func TestFilterTasksReadyAndSearch(t *testing.T) {
	t.Parallel()

	tasks := []Task{
		{ID: "wl-done", Title: "Completed blocker", Status: "done", Kind: "task", Priority: "normal"},
		{ID: "wl-ready", Title: "Fix web editor", Status: "todo", Kind: "bug", Priority: "high", Tags: []string{"editor"}, BlockedBy: []string{"wl-done"}, Repositories: []RepositoryScope{{ID: "github.com/acme/web"}}},
		{ID: "wl-blocked", Title: "Wait for API", Status: "todo", Kind: "task", Priority: "normal", BlockedBy: []string{"wl-missing"}},
	}

	ready := FilterTasks(tasks, Filter{Ready: true})
	if len(ready) != 1 || ready[0].ID != "wl-ready" {
		t.Errorf("ready tasks = %#v", ready)
	}
	matching := FilterTasks(tasks, Filter{Repository: "github.com/acme/web", Query: "editor", Tags: []string{"editor"}})
	if len(matching) != 1 || matching[0].ID != "wl-ready" {
		t.Errorf("matching tasks = %#v", matching)
	}
}
