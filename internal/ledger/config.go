package ledger

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

const ConfigSchema = "work-ledger/config/v1"

// Config is the durable configuration for one private ledger directory.
type Config struct {
	Schema            string              `yaml:"schema"`
	RepositoryAliases map[string][]string `yaml:"repository_aliases,omitempty"`
}

// Paths collects every path owned by a ledger. All durable paths stay under
// Home so Work Ledger never writes to the source repository it is run from.
type Paths struct {
	Home     string
	Config   string
	Tasks    string
	Archive  string
	Runtime  string
	Locks    string
	Template string
}

func NewPaths(home string) Paths {
	cleanHome := filepath.Clean(home)
	return Paths{
		Home:     cleanHome,
		Config:   filepath.Join(cleanHome, "ledger.yaml"),
		Tasks:    filepath.Join(cleanHome, "tasks"),
		Archive:  filepath.Join(cleanHome, "archive"),
		Runtime:  filepath.Join(cleanHome, ".runtime"),
		Locks:    filepath.Join(cleanHome, ".runtime", "locks"),
		Template: filepath.Join(cleanHome, "templates"),
	}
}

func DefaultHome(userHome, configuredHome string) (string, error) {
	if configuredHome != "" {
		return filepath.Abs(configuredHome)
	}
	if userHome == "" {
		return "", errors.New("could not determine a home directory; set WORK_LEDGER_HOME or pass --home")
	}
	return filepath.Join(userHome, ".work-ledger"), nil
}

// Initialize creates a new ledger if it does not exist. It is safe to run more
// than once: an existing ledger is validated but never overwritten.
func Initialize(paths Paths) (created bool, err error) {
	for _, directory := range []string{paths.Home, paths.Tasks, paths.Archive, paths.Runtime, paths.Locks, paths.Template} {
		if err := os.MkdirAll(directory, 0o700); err != nil {
			return false, fmt.Errorf("create %s: %w", directory, err)
		}
	}

	if _, err := os.Stat(paths.Config); err == nil {
		_, err := Load(paths)
		return false, err
	} else if !errors.Is(err, os.ErrNotExist) {
		return false, fmt.Errorf("inspect %s: %w", paths.Config, err)
	}

	config := Config{Schema: ConfigSchema, RepositoryAliases: map[string][]string{}}
	contents, err := yaml.Marshal(config)
	if err != nil {
		return false, fmt.Errorf("encode ledger configuration: %w", err)
	}
	if err := writeFileAtomically(paths.Config, contents, 0o600); err != nil {
		return false, err
	}

	return true, nil
}

func Load(paths Paths) (Config, error) {
	contents, err := os.ReadFile(paths.Config)
	if err != nil {
		return Config{}, fmt.Errorf("read %s: %w", paths.Config, err)
	}

	var config Config
	if err := yaml.Unmarshal(contents, &config); err != nil {
		return Config{}, fmt.Errorf("parse %s: %w", paths.Config, err)
	}
	if err := config.Validate(); err != nil {
		return Config{}, fmt.Errorf("validate %s: %w", paths.Config, err)
	}
	return config, nil
}

func (config Config) Validate() error {
	if config.Schema != ConfigSchema {
		return fmt.Errorf("schema must be %q, got %q", ConfigSchema, config.Schema)
	}

	for canonical, aliases := range config.RepositoryAliases {
		if strings.TrimSpace(canonical) == "" {
			return errors.New("repository aliases contain an empty canonical repository ID")
		}
		for _, alias := range aliases {
			if strings.TrimSpace(alias) == "" {
				return fmt.Errorf("repository %q has an empty alias", canonical)
			}
		}
	}
	return nil
}

func writeFileAtomically(path string, contents []byte, mode os.FileMode) error {
	temporary, err := os.CreateTemp(filepath.Dir(path), ".work-ledger-*")
	if err != nil {
		return fmt.Errorf("create temporary file for %s: %w", path, err)
	}
	temporaryPath := temporary.Name()
	defer os.Remove(temporaryPath)

	if err := temporary.Chmod(mode); err != nil {
		temporary.Close()
		return fmt.Errorf("set permissions on %s: %w", temporaryPath, err)
	}
	if _, err := temporary.Write(contents); err != nil {
		temporary.Close()
		return fmt.Errorf("write %s: %w", temporaryPath, err)
	}
	if err := temporary.Sync(); err != nil {
		temporary.Close()
		return fmt.Errorf("sync %s: %w", temporaryPath, err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close %s: %w", temporaryPath, err)
	}
	if err := os.Rename(temporaryPath, path); err != nil {
		return fmt.Errorf("replace %s: %w", path, err)
	}
	return nil
}
