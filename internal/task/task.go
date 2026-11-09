package task

import (
	"crypto/rand"
	"encoding/base32"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"
	"unicode"

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
	Now        time.Time
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
	Parent       string            `json:"parent,omitempty"`
	Claim        *Claim            `json:"claim,omitempty"`
	Stale        bool              `json:"stale,omitempty"`
	UpdatedAt    string            `json:"updated_at"`
	Path         string            `json:"path,omitempty"`
}

func New(title, kind, priority string, repositories []RepositoryScope, now time.Time) (Task, error) {
	if kind == "" {
		kind = "task"
	}
	if priority == "" {
		priority = "normal"
	}
	timestamp := now.UTC().Format(time.RFC3339)
	task := Task{
		Schema:       Schema,
		ID:           slugID(title),
		Title:        strings.TrimSpace(title),
		Kind:         kind,
		Status:       "todo",
		Priority:     priority,
		Repositories: deduplicateRepositories(repositories),
		CreatedAt:    timestamp,
		UpdatedAt:    timestamp,
	}
	if err := task.Validate(); err != nil {
		return Task{}, err
	}
	return task, nil
}

// Create writes a new task file. When the task's ID is in taken or already has
// a file, a short random suffix is appended until the ID is free; taken is
// updated with the final ID.
func Create(directory string, task Task, taken map[string]bool) (Task, error) {
	base := task.ID
	for attempt := 0; ; attempt++ {
		if attempt > 0 {
			suffix, err := randomSuffix()
			if err != nil {
				return Task{}, err
			}
			task.ID = base + "-" + suffix
		}
		if taken[task.ID] {
			continue
		}
		created, err := createFile(directory, task)
		if errors.Is(err, os.ErrExist) && attempt < 8 {
			continue
		}
		if err != nil {
			return Task{}, err
		}
		if taken != nil {
			taken[created.ID] = true
		}
		return created, nil
	}
}

func createFile(directory string, task Task) (Task, error) {
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

// TakenIDs returns every ID and legacy ID in tasks, for Create.
func TakenIDs(tasks []Task) map[string]bool {
	taken := make(map[string]bool, len(tasks))
	for _, task := range tasks {
		taken[task.ID] = true
		for _, legacyID := range task.LegacyIDs {
			taken[legacyID] = true
		}
	}
	return taken
}

func Render(task Task) ([]byte, error) {
	if err := task.Validate(); err != nil {
		return nil, err
	}
	frontmatter, err := yaml.Marshal(task)
	if err != nil {
		return nil, fmt.Errorf("encode task frontmatter: %w", err)
	}
	body := strings.TrimSpace(task.Body)
	if body != "" {
		body = "\n" + body + "\n"
	}
	return []byte("---\n" + string(frontmatter) + "---\n\n# " + task.Title + "\n" + body), nil
}

// FileName is <id>.md. Tasks created before slug IDs are named
// <id>-<title-slug>.md and keep that name.
func FileName(task Task) string {
	return task.ID + ".md"
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

// Find resolves an exact ID or legacy ID, then a unique ID prefix.
func Find(tasks []Task, identifier string) (Task, error) {
	identifier = strings.TrimSpace(identifier)
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
	var matches []Task
	if identifier != "" {
		for _, task := range tasks {
			if strings.HasPrefix(task.ID, identifier) {
				matches = append(matches, task)
			}
		}
	}
	switch len(matches) {
	case 0:
		return Task{}, fmt.Errorf("task %q was not found", identifier)
	case 1:
		return matches[0], nil
	}
	ids := make([]string, 0, len(matches))
	for _, match := range matches {
		ids = append(ids, match.ID)
	}
	return Task{}, fmt.Errorf("task %q is ambiguous: %s", identifier, strings.Join(ids, ", "))
}

func FilterTasks(tasks []Task, filter Filter) []Task {
	if filter.Now.IsZero() {
		filter.Now = time.Now()
	}
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
		if filter.Ready && !task.IsReady(byID) && !task.IsStale(filter.Now) {
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
			if filename != FileName(task) && (!strings.HasPrefix(filename, task.ID+"-") || filepath.Ext(filename) != ".md") {
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
		Parent:       task.Parent,
		Claim:        task.Claim,
		Stale:        task.IsStale(time.Now()),
		UpdatedAt:    task.UpdatedAt,
		Path:         task.Path,
	}
}

// IsStale reports an active task whose claim has expired, or that has no
// claim: the session that held it stopped without releasing or closing it.
func (task Task) IsStale(now time.Time) bool {
	if task.Status != "active" {
		return false
	}
	return !task.Claim.Held(now)
}

// Held reports whether the claim exists and has not expired.
func (claim *Claim) Held(now time.Time) bool {
	if claim == nil {
		return false
	}
	expiresAt, err := time.Parse(time.RFC3339, claim.ExpiresAt)
	return err == nil && expiresAt.After(now)
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

const (
	maxSlugWords  = 6
	maxSlugLength = 48
)

// slugID turns a title into a short, typeable ID: lowercase ASCII words
// joined by hyphens, cut at a word boundary.
func slugID(title string) string {
	var words []string
	var word strings.Builder
	flush := func() {
		if word.Len() > 0 {
			words = append(words, word.String())
			word.Reset()
		}
	}
	for _, character := range strings.ToLower(title) {
		if character < unicode.MaxASCII && (unicode.IsLetter(character) || unicode.IsDigit(character)) {
			word.WriteRune(character)
			continue
		}
		flush()
	}
	flush()

	var slug string
	for index, word := range words {
		if index == maxSlugWords {
			break
		}
		candidate := word
		if slug != "" {
			candidate = slug + "-" + word
		}
		if len(candidate) > maxSlugLength {
			if slug == "" {
				slug = word[:maxSlugLength]
			}
			break
		}
		slug = candidate
	}
	if slug == "" {
		return "task"
	}
	return slug
}

var suffixEncoding = base32.NewEncoding("0123456789abcdefghjkmnpqrstvwxyz").WithPadding(base32.NoPadding)

func randomSuffix() (string, error) {
	buffer := make([]byte, 3)
	if _, err := rand.Read(buffer); err != nil {
		return "", fmt.Errorf("generate task ID suffix: %w", err)
	}
	return suffixEncoding.EncodeToString(buffer)[:4], nil
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
