package importer

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/NelsonBrandao/work-ledger/internal/ledger"
	"github.com/NelsonBrandao/work-ledger/internal/store"
	"github.com/NelsonBrandao/work-ledger/internal/task"
	"gopkg.in/yaml.v3"
)

type Result struct {
	DryRun   bool           `json:"dry_run"`
	Imported int            `json:"imported"`
	Tasks    []task.Summary `json:"tasks,omitempty"`
}

type beansRecord struct {
	ID        string
	Title     string   `yaml:"title"`
	Status    string   `yaml:"status"`
	Type      string   `yaml:"type"`
	Priority  string   `yaml:"priority"`
	Parent    string   `yaml:"parent"`
	BlockedBy []string `yaml:"blocked_by"`
	Tags      []string `yaml:"tags"`
	CreatedAt string   `yaml:"created_at"`
	UpdatedAt string   `yaml:"updated_at"`
	Body      string
}

func Beans(paths ledger.Paths, source string, repositories []task.RepositoryScope, dryRun bool) (Result, error) {
	records, err := readBeans(source)
	if err != nil {
		return Result{}, err
	}
	if dryRun {
		return Result{DryRun: true, Imported: len(records)}, nil
	}

	existing, err := task.LoadAll(paths.Tasks, paths.Archive)
	if err != nil {
		return Result{}, err
	}
	for _, record := range records {
		if hasLegacyID(existing, record.ID) {
			return Result{}, fmt.Errorf("a task with legacy Beans ID %q already exists", record.ID)
		}
	}

	taken := task.TakenIDs(existing)
	createdByLegacyID := make(map[string]task.Task, len(records))
	result := Result{Imported: len(records)}
	for _, record := range records {
		createdAt := parseTime(record.CreatedAt)
		created, err := task.New(record.Title, mapKind(record.Type), mapPriority(record.Priority), repositories, createdAt)
		if err != nil {
			return Result{}, fmt.Errorf("create task for Beans ID %s: %w", record.ID, err)
		}
		created.Status = mapStatus(record.Status)
		created.UpdatedAt = parseTime(record.UpdatedAt).UTC().Format(time.RFC3339)
		created.CreatedAt = createdAt.UTC().Format(time.RFC3339)
		created.Tags = record.Tags
		created.LegacyIDs = []string{record.ID}
		created.Body = record.Body
		created, err = task.Create(paths.Tasks, created, taken)
		if err != nil {
			return Result{}, err
		}
		createdByLegacyID[record.ID] = created
		result.Tasks = append(result.Tasks, created.Summary())
	}

	for _, record := range records {
		created := createdByLegacyID[record.ID]
		_, err := store.MutateAt(paths, created.ID, "", parseTime(record.UpdatedAt), func(document *task.Document) error {
			if record.Parent != "" {
				if parent, found := createdByLegacyID[record.Parent]; found {
					document.Parent = parent.ID
				}
			}
			for _, sourceBlocker := range record.BlockedBy {
				if blocker, found := createdByLegacyID[sourceBlocker]; found {
					document.BlockedBy = append(document.BlockedBy, blocker.ID)
				}
			}
			return nil
		})
		if err != nil {
			return Result{}, err
		}
	}
	return result, nil
}

func readBeans(source string) ([]beansRecord, error) {
	info, err := os.Stat(source)
	if err != nil {
		return nil, fmt.Errorf("inspect Beans path: %w", err)
	}
	if !info.IsDir() {
		return nil, errors.New("Beans import path must be a directory")
	}

	var records []beansRecord
	err = filepath.WalkDir(source, func(path string, entry os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if entry.IsDir() || filepath.Ext(path) != ".md" {
			return nil
		}
		record, err := readBean(path)
		if err != nil {
			return err
		}
		records = append(records, record)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("read Beans directory: %w", err)
	}
	sort.Slice(records, func(left, right int) bool { return records[left].ID < records[right].ID })
	return records, nil
}

func readBean(path string) (beansRecord, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return beansRecord{}, err
	}
	frontmatter, body, err := splitFrontmatter(string(contents))
	if err != nil {
		return beansRecord{}, fmt.Errorf("parse %s: %w", path, err)
	}
	var record beansRecord
	if err := yaml.Unmarshal([]byte(frontmatter), &record); err != nil {
		return beansRecord{}, fmt.Errorf("decode %s: %w", path, err)
	}
	record.ID = headerID(frontmatter)
	if record.ID == "" {
		return beansRecord{}, fmt.Errorf("parse %s: Beans ID comment is missing", path)
	}
	if strings.TrimSpace(record.Title) == "" {
		return beansRecord{}, fmt.Errorf("parse %s: title is missing", path)
	}
	record.Body = body
	return record, nil
}

func splitFrontmatter(contents string) (string, string, error) {
	normalized := strings.ReplaceAll(contents, "\r\n", "\n")
	if !strings.HasPrefix(normalized, "---\n") {
		return "", "", errors.New("file must start with YAML frontmatter")
	}
	remainder := normalized[len("---\n"):]
	separator := strings.Index(remainder, "\n---\n")
	if separator < 0 {
		return "", "", errors.New("YAML frontmatter is missing a closing --- delimiter")
	}
	return remainder[:separator], remainder[separator+len("\n---\n"):], nil
}

func headerID(frontmatter string) string {
	for _, line := range strings.Split(frontmatter, "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "#") {
			return strings.TrimSpace(strings.TrimPrefix(trimmed, "#"))
		}
	}
	return ""
}

func parseTime(value string) time.Time {
	parsed, err := time.Parse(time.RFC3339, value)
	if err != nil {
		return time.Now().UTC()
	}
	return parsed
}

func mapStatus(value string) string {
	switch value {
	case "completed":
		return "done"
	case "scrapped":
		return "cancelled"
	default:
		return "todo"
	}
}

func mapKind(value string) string {
	switch value {
	case "bug", "feature", "investigation":
		return value
	default:
		return "task"
	}
}

func mapPriority(value string) string {
	switch value {
	case "critical", "high", "normal", "low":
		return value
	default:
		return "normal"
	}
}

func hasLegacyID(tasks []task.Task, target string) bool {
	for _, candidate := range tasks {
		for _, legacyID := range candidate.LegacyIDs {
			if legacyID == target {
				return true
			}
		}
	}
	return false
}
