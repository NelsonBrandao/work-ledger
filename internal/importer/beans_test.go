package importer

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/NelsonBrandao/work-ledger/internal/ledger"
	"github.com/NelsonBrandao/work-ledger/internal/task"
)

func TestBeansDryRunAndImport(t *testing.T) {
	paths := ledger.NewPaths(filepath.Join(t.TempDir(), "ledger"))
	if _, err := ledger.Initialize(paths); err != nil {
		t.Fatal(err)
	}
	beansDirectory := filepath.Join(t.TempDir(), ".beans")
	if err := os.MkdirAll(beansDirectory, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(beansDirectory, "parent.md"), []byte(bean("parcel-parent", "Parent", "todo", "", nil)), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(beansDirectory, "child.md"), []byte(bean("parcel-child", "Child", "completed", "parcel-parent", []string{"parcel-parent"})), 0o600); err != nil {
		t.Fatal(err)
	}

	dryRun, err := Beans(paths, beansDirectory, nil, true)
	if err != nil {
		t.Fatal(err)
	}
	if !dryRun.DryRun || dryRun.Imported != 2 {
		t.Errorf("dry run = %#v", dryRun)
	}

	result, err := Beans(paths, beansDirectory, nil, false)
	if err != nil {
		t.Fatal(err)
	}
	if result.Imported != 2 || len(result.Tasks) != 2 {
		t.Errorf("result = %#v", result)
	}
	imported, err := task.LoadAll(paths.Tasks)
	if err != nil {
		t.Fatal(err)
	}
	parent, err := task.Find(imported, "parcel-parent")
	if err != nil {
		t.Fatal(err)
	}
	child, err := task.Find(imported, "parcel-child")
	if err != nil {
		t.Fatal(err)
	}
	if child.Parent != parent.ID || len(child.BlockedBy) != 1 || child.BlockedBy[0] != parent.ID {
		t.Errorf("child relationships = %#v, parent = %#v", child, parent)
	}
	if child.UpdatedAt != "2026-07-15T14:00:00Z" {
		t.Errorf("child updated_at = %q", child.UpdatedAt)
	}
}

func bean(id, title, status, parent string, blockers []string) string {
	blockerLines := ""
	for _, blocker := range blockers {
		blockerLines += "  - " + blocker + "\n"
	}
	parentLine := ""
	if parent != "" {
		parentLine = "parent: " + parent + "\n"
	}
	return "---\n# " + id + "\ntitle: " + title + "\nstatus: " + status + "\ntype: task\ncreated_at: 2026-07-15T14:00:00Z\nupdated_at: 2026-07-15T14:00:00Z\n" + parentLine + "blocked_by:\n" + blockerLines + "---\n\n# " + title + "\n\nInvestigation\n"
}
