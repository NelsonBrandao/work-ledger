package task

import (
	"crypto/rand"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

	"github.com/oklog/ulid/v2"
	"gopkg.in/yaml.v3"
)

const Schema = "work-ledger/task/v1"

var validStatuses = map[string]bool{
	"todo":      true,
	"active":    true,
	"blocked":   true,
	"done":      true,
	"cancelled": true,
}

var validKinds = map[string]bool{
	"task":          true,
	"bug":           true,
	"investigation": true,
	"feature":       true,
}

var validPriorities = map[string]bool{
	"critical": true,
	"high":     true,
	"normal":   true,
	"low":      true,
}

// RepositoryScope connects a task to a repository and, optionally, a subset of
// paths. A task can contain any number of scopes.
type RepositoryScope struct {
	ID    string   `yaml:"id" json:"id"`
	Paths []string `yaml:"paths,omitempty" json:"paths,omitempty"`
}

type Claim struct {
	Agent     string `yaml:"agent" json:"agent"`
	ExpiresAt string `yaml:"expires_at" json:"expires_at"`
}

// Task contains only the metadata that the first read-path commands need. The
// unmodified Markdown body is retained separately for future write commands.
type Task struct {
	Schema       string            `yaml:"schema" json:"schema"`
	ID           string            `yaml:"id" json:"id"`
	Title        string            `yaml:"title" json:"title"`
	Kind         string            `yaml:"kind" json:"kind"`
	Status       string            `yaml:"status" json:"status"`
	Priority     string            `yaml:"priority" json:"priority"`
	Repositories []RepositoryScope `yaml:"repositories" json:"repositories"`
	Parent       string            `yaml:"parent,omitempty" json:"parent,omitempty"`
	BlockedBy    []string          `yaml:"blocked_by,omitempty" json:"blocked_by,omitempty"`
	Tags         []string          `yaml:"tags,omitempty" json:"tags,omitempty"`
	Claim        *Claim            `yaml:"claim,omitempty" json:"claim,omitempty"`
	CreatedAt    string            `yaml:"created_at" json:"created_at"`
	UpdatedAt    string            `yaml:"updated_at" json:"updated_at"`
	LegacyIDs    []string          `yaml:"legacy_ids,omitempty" json:"legacy_ids,omitempty"`
	Body         string            `yaml:"-" json:"body,omitempty"`
	Path         string            `yaml:"-" json:"path,omitempty"`
	ETag         string            `yaml:"-" json:"etag,omitempty"`
}

type Filter struct {
	Repository string
	Parent     string
	Statuses   []string
	Tags       []string
	Ready      bool
	Query      string
}

// Summary is the compact task view returned by list and search. It avoids
// sending long investigations to agents that only need to choose work.
type Summary struct {
	ID           string            `json:"id"`
	Title        string            `json:"title"`
	Kind         string            `json:"kind"`
	Status       string            `json:"status"`
	Priority     string            `json:"priority"`
	Repositories []RepositoryScope `json:"repositories"`
	Tags         []string          `json:"tags,omitempty"`
	UpdatedAt    string            `json:"updated_at"`
}

func New(title, kind, priority string, repositories []RepositoryScope, now time.Time) (Task, error) {
	if kind == "" {
		kind = "task"
	}
	if priority == "" {
		priority = "normal"
	}
	identifier, err := ulid.New(ulid.Timestamp(now), rand.Reader)
	if err != nil {
		return Task{}, fmt.Errorf("generate task ID: %w", err)
	}
	timestamp := now.UTC().Format(time.RFC3339)
	task := Task{
		Schema:       Schema,
		ID:           "wl-" + strings.ToLower(identifier.String()),
		Title:        strings.TrimSpace(title),
		Kind:         kind,
		Status:       "todo",
		Priority:     priority,
		Repositories: deduplicateRepositories(repositories),
		CreatedAt:    timestamp,
		UpdatedAt:    timestamp,
		Body:         defaultBody(),
	}
	if err := task.Validate(); err != nil {
		return Task{}, err
	}
	return task, nil
}

func Create(directory string, task Task) (Task, error) {
	contents, err := Render(task)
	if err != nil {
		return Task{}, err
	}
	path := filepath.Join(directory, FileName(task))
	file, err := os.OpenFile(path, os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o600)
	if err != nil {
		return Task{}, fmt.Errorf("create %s: %w", path, err)
	}
	if _, err := file.Write(contents); err != nil {
		file.Close()
		return Task{}, fmt.Errorf("write %s: %w", path, err)
	}
	if err := file.Close(); err != nil {
		return Task{}, fmt.Errorf("close %s: %w", path, err)
	}
	task.Path = path
	return task, nil
}

func Render(task Task) ([]byte, error) {
	if err := task.Validate(); err != nil {
		return nil, err
	}
	frontmatter, err := yaml.Marshal(task)
	if err != nil {
		return nil, fmt.Errorf("encode task frontmatter: %w", err)
	}
	body := task.Body
	if body == "" {
		body = defaultBody()
	}
	return []byte("---\n" + string(frontmatter) + "---\n\n# " + task.Title + "\n\n" + strings.TrimPrefix(body, "\n")), nil
}

func FileName(task Task) string {
	return task.ID + "-" + slugify(task.Title) + ".md"
}

func Load(path string) (Task, error) {
	document, err := LoadDocument(path)
	if err != nil {
		return Task{}, err
	}
	return document.Task, nil
}

func Parse(contents string) (Task, error) {
	document, err := ParseDocument(contents)
	if err != nil {
		return Task{}, err
	}
	return document.Task, nil
}

func LoadAll(directories ...string) ([]Task, error) {
	var tasks []Task
	for _, directory := range directories {
		err := filepath.WalkDir(directory, func(path string, entry os.DirEntry, err error) error {
			if err != nil {
				return err
			}
			if entry.IsDir() || filepath.Ext(path) != ".md" {
				return nil
			}
			task, err := Load(path)
			if err != nil {
				return err
			}
			tasks = append(tasks, task)
			return nil
		})
		if err != nil {
			return nil, fmt.Errorf("scan %s: %w", directory, err)
		}
	}
	sort.Slice(tasks, func(left, right int) bool {
		return tasks[left].ID < tasks[right].ID
	})
	return tasks, nil
}

func Find(tasks []Task, identifier string) (Task, error) {
	for _, task := range tasks {
		if task.ID == identifier {
			return task, nil
		}
		for _, legacyID := range task.LegacyIDs {
			if legacyID == identifier {
				return task, nil
			}
		}
	}
	return Task{}, fmt.Errorf("task %q was not found", identifier)
}

func FilterTasks(tasks []Task, filter Filter) []Task {
	byID := make(map[string]Task, len(tasks))
	for _, task := range tasks {
		byID[task.ID] = task
	}

	var matching []Task
	for _, task := range tasks {
		if filter.Repository != "" && !task.HasRepository(filter.Repository) {
			continue
		}
		if filter.Parent != "" && task.Parent != filter.Parent {
			continue
		}
		if len(filter.Statuses) > 0 && !contains(filter.Statuses, task.Status) {
			continue
		}
		if len(filter.Tags) > 0 && !sharesValue(task.Tags, filter.Tags) {
			continue
		}
		if filter.Ready && !task.IsReady(byID) {
			continue
		}
		if filter.Query != "" && !strings.Contains(strings.ToLower(task.searchableText()), strings.ToLower(filter.Query)) {
			continue
		}
		matching = append(matching, task)
	}
	return matching
}

func ValidateCollection(tasks []Task) error {
	byID := make(map[string]Task, len(tasks))
	for _, task := range tasks {
		if _, exists := byID[task.ID]; exists {
			return fmt.Errorf("task ID %q appears more than once", task.ID)
		}
		if task.Path != "" {
			filename := filepath.Base(task.Path)
			if !strings.HasPrefix(filename, task.ID+"-") || filepath.Ext(filename) != ".md" {
				return fmt.Errorf("task %q has an invalid filename %q", task.ID, filename)
			}
		}
		byID[task.ID] = task
	}
	for _, task := range tasks {
		if task.Parent != "" {
			if _, exists := byID[task.Parent]; !exists {
				return fmt.Errorf("task %q references missing parent %q", task.ID, task.Parent)
			}
		}
		for _, blockerID := range task.BlockedBy {
			if _, exists := byID[blockerID]; !exists {
				return fmt.Errorf("task %q references missing blocker %q", task.ID, blockerID)
			}
		}
	}
	return nil
}

func (task Task) HasRepository(repositoryID string) bool {
	for _, repository := range task.Repositories {
		if repository.ID == repositoryID {
			return true
		}
	}
	return false
}

func (task Task) Summary() Summary {
	return Summary{
		ID:           task.ID,
		Title:        task.Title,
		Kind:         task.Kind,
		Status:       task.Status,
		Priority:     task.Priority,
		Repositories: task.Repositories,
		Tags:         task.Tags,
		UpdatedAt:    task.UpdatedAt,
	}
}

func (task Task) IsReady(tasksByID map[string]Task) bool {
	if task.Status != "todo" {
		return false
	}
	for _, blockerID := range task.BlockedBy {
		blocker, exists := tasksByID[blockerID]
		if !exists || (blocker.Status != "done" && blocker.Status != "cancelled") {
			return false
		}
	}
	return true
}

func (task Task) Validate() error {
	if task.Schema != Schema {
		return fmt.Errorf("schema must be %q, got %q", Schema, task.Schema)
	}
	if strings.TrimSpace(task.ID) == "" {
		return errors.New("id is required")
	}
	if strings.TrimSpace(task.Title) == "" {
		return errors.New("title is required")
	}
	if !validStatuses[task.Status] {
		return fmt.Errorf("status %q is invalid", task.Status)
	}
	if !validKinds[task.Kind] {
		return fmt.Errorf("kind %q is invalid", task.Kind)
	}
	if !validPriorities[task.Priority] {
		return fmt.Errorf("priority %q is invalid", task.Priority)
	}
	for _, repository := range task.Repositories {
		if strings.TrimSpace(repository.ID) == "" {
			return errors.New("repositories contain an empty ID")
		}
	}
	return nil
}

func splitFrontmatter(contents string) (frontmatter, body string, err error) {
	normalized := strings.ReplaceAll(contents, "\r\n", "\n")
	if !strings.HasPrefix(normalized, "---\n") {
		return "", "", errors.New("task must start with YAML frontmatter delimited by ---")
	}

	remainder := normalized[len("---\n"):]
	separator := strings.Index(remainder, "\n---\n")
	if separator < 0 {
		return "", "", errors.New("task YAML frontmatter is missing a closing --- delimiter")
	}
	return remainder[:separator], remainder[separator+len("\n---\n"):], nil
}

func defaultBody() string {
	return "## Plan\n\n- [ ]\n\n## Investigation\n\n## Activity\n"
}

func deduplicateRepositories(repositories []RepositoryScope) []RepositoryScope {
	seen := make(map[string]bool, len(repositories))
	var unique []RepositoryScope
	for _, repository := range repositories {
		if seen[repository.ID] {
			continue
		}
		seen[repository.ID] = true
		unique = append(unique, repository)
	}
	return unique
}

func slugify(value string) string {
	var builder strings.Builder
	previousHyphen := false
	for _, character := range strings.ToLower(value) {
		if unicode.IsLetter(character) || unicode.IsDigit(character) {
			builder.WriteRune(character)
			previousHyphen = false
			continue
		}
		if builder.Len() > 0 && !previousHyphen {
			builder.WriteByte('-')
			previousHyphen = true
		}
	}
	slug := strings.Trim(builder.String(), "-")
	if slug == "" {
		return "task"
	}
	return slug
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}

func sharesValue(left, right []string) bool {
	for _, value := range left {
		if contains(right, value) {
			return true
		}
	}
	return false
}

func (task Task) searchableText() string {
	return strings.Join([]string{task.Title, task.Kind, task.Status, task.Priority, strings.Join(task.Tags, " "), task.Body}, " ")
}
