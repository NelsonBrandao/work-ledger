package ledger

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDefaultHome(t *testing.T) {
	t.Parallel()

	home, err := DefaultHome("/Users/example", "")
	if err != nil {
		t.Fatalf("DefaultHome returned an error: %v", err)
	}
	if want := "/Users/example/.work-ledger"; home != want {
		t.Errorf("DefaultHome = %q, want %q", home, want)
	}
}

func TestDefaultHomeUsesConfiguredLocation(t *testing.T) {
	t.Parallel()

	home, err := DefaultHome("/Users/example", "/private/tmp/custom-ledger")
	if err != nil {
		t.Fatalf("DefaultHome returned an error: %v", err)
	}
	if want := "/private/tmp/custom-ledger"; home != want {
		t.Errorf("DefaultHome = %q, want %q", home, want)
	}
}

func TestInitializeCreatesAndValidatesLedger(t *testing.T) {
	t.Parallel()

	paths := NewPaths(filepath.Join(t.TempDir(), "ledger"))
	created, err := Initialize(paths)
	if err != nil {
		t.Fatalf("Initialize returned an error: %v", err)
	}
	if !created {
		t.Fatal("Initialize reported an existing ledger")
	}

	for _, path := range []string{paths.Tasks, paths.Archive, paths.Locks, paths.Template} {
		if info, err := os.Stat(path); err != nil || !info.IsDir() {
			t.Errorf("%s was not created as a directory: %v", path, err)
		}
	}

	config, err := Load(paths)
	if err != nil {
		t.Fatalf("Load returned an error: %v", err)
	}
	if config.Schema != ConfigSchema {
		t.Errorf("config schema = %q, want %q", config.Schema, ConfigSchema)
	}

	created, err = Initialize(paths)
	if err != nil {
		t.Fatalf("second Initialize returned an error: %v", err)
	}
	if created {
		t.Error("second Initialize reported a newly created ledger")
	}
}

func TestLoadRejectsInvalidConfiguration(t *testing.T) {
	t.Parallel()

	paths := NewPaths(t.TempDir())
	if err := os.WriteFile(paths.Config, []byte("schema: unexpected/v1\n"), 0o600); err != nil {
		t.Fatal(err)
	}

	if _, err := Load(paths); err == nil {
		t.Fatal("Load succeeded for an invalid configuration")
	}
}
