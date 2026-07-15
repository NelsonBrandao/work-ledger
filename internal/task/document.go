package task

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Document preserves the parsed frontmatter tree alongside the typed task. The
// tree lets mutations retain user-owned keys and comments that the CLI does not
// understand yet.
type Document struct {
	Task
	frontmatter yaml.Node
}

func LoadDocument(path string) (Document, error) {
	contents, err := os.ReadFile(path)
	if err != nil {
		return Document{}, fmt.Errorf("read %s: %w", path, err)
	}
	document, err := ParseDocument(string(contents))
	if err != nil {
		return Document{}, fmt.Errorf("parse %s: %w", path, err)
	}
	document.Path = path
	document.ETag = contentHash(contents)
	return document, nil
}

func ParseDocument(contents string) (Document, error) {
	frontmatter, body, err := splitFrontmatter(contents)
	if err != nil {
		return Document{}, err
	}

	var node yaml.Node
	if err := yaml.Unmarshal([]byte(frontmatter), &node); err != nil {
		return Document{}, fmt.Errorf("decode YAML frontmatter: %w", err)
	}
	var task Task
	if err := node.Decode(&task); err != nil {
		return Document{}, fmt.Errorf("decode task frontmatter: %w", err)
	}
	task.Body = body
	if err := task.Validate(); err != nil {
		return Document{}, err
	}
	return Document{Task: task, frontmatter: node}, nil
}

func (document *Document) Save(now time.Time) error {
	document.UpdatedAt = now.UTC().Format(time.RFC3339)
	document.syncFrontmatter()
	contents, err := document.render()
	if err != nil {
		return err
	}
	if err := writeFileAtomically(document.Path, contents, 0o600); err != nil {
		return err
	}
	document.ETag = contentHash(contents)
	return nil
}

func (document *Document) AppendActivity(now time.Time, agent, message string) {
	entry := fmt.Sprintf("- %s — %s: %s\n", now.UTC().Format(time.RFC3339), agent, strings.TrimSpace(message))
	activity := "## Activity\n"
	index := strings.Index(document.Body, activity)
	if index < 0 {
		if !strings.HasSuffix(document.Body, "\n") {
			document.Body += "\n"
		}
		document.Body += "\n" + activity + "\n" + entry
		return
	}
	insertAt := index + len(activity)
	if insertAt < len(document.Body) && document.Body[insertAt] == '\n' {
		insertAt++
	}
	document.Body = document.Body[:insertAt] + entry + document.Body[insertAt:]
}

func (document *Document) RenameTitle(title string) {
	oldTitle := document.Title
	document.Title = title
	oldHeading := "# " + oldTitle
	if index := strings.Index(document.Body, oldHeading); index >= 0 {
		document.Body = document.Body[:index] + "# " + title + document.Body[index+len(oldHeading):]
	}
}

func (document *Document) SetClaim(claim *Claim) {
	document.Claim = claim
}

func (document *Document) syncFrontmatter() {
	document.setString("schema", document.Schema)
	document.setString("id", document.ID)
	document.setString("title", document.Title)
	document.setString("kind", document.Kind)
	document.setString("status", document.Status)
	document.setString("priority", document.Priority)
	document.setNode("repositories", document.Repositories, false)
	document.setOptionalString("parent", document.Parent)
	document.setNode("blocked_by", document.BlockedBy, len(document.BlockedBy) == 0)
	document.setNode("tags", document.Tags, len(document.Tags) == 0)
	document.setNode("claim", document.Claim, document.Claim == nil)
	document.setString("created_at", document.CreatedAt)
	document.setString("updated_at", document.UpdatedAt)
	document.setNode("legacy_ids", document.LegacyIDs, len(document.LegacyIDs) == 0)
}

func (document *Document) setString(key, value string) {
	document.setNode(key, value, false)
}

func (document *Document) setOptionalString(key, value string) {
	document.setNode(key, value, value == "")
}

func (document *Document) setNode(key string, value any, remove bool) {
	mapping := document.mapping()
	index := mappingIndex(mapping, key)
	if remove {
		if index >= 0 {
			mapping.Content = append(mapping.Content[:index], mapping.Content[index+2:]...)
		}
		return
	}

	encoded, err := yaml.Marshal(value)
	if err != nil {
		panic(fmt.Sprintf("encode task field %q: %v", key, err))
	}
	var node yaml.Node
	if err := yaml.Unmarshal(encoded, &node); err != nil {
		panic(fmt.Sprintf("decode task field %q: %v", key, err))
	}
	valueNode := node.Content[0]
	if index >= 0 {
		mapping.Content[index+1] = valueNode
		return
	}
	mapping.Content = append(mapping.Content,
		&yaml.Node{Kind: yaml.ScalarNode, Tag: "!!str", Value: key},
		valueNode,
	)
}

func (document *Document) mapping() *yaml.Node {
	if document.frontmatter.Kind == yaml.DocumentNode {
		return document.frontmatter.Content[0]
	}
	return &document.frontmatter
}

func (document *Document) render() ([]byte, error) {
	if err := document.Task.Validate(); err != nil {
		return nil, err
	}
	encoded, err := yaml.Marshal(&document.frontmatter)
	if err != nil {
		return nil, fmt.Errorf("encode task frontmatter: %w", err)
	}
	return []byte("---\n" + string(encoded) + "---\n" + document.Body), nil
}

func mappingIndex(mapping *yaml.Node, key string) int {
	for index := 0; index+1 < len(mapping.Content); index += 2 {
		if mapping.Content[index].Value == key {
			return index
		}
	}
	return -1
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

func contentHash(contents []byte) string {
	sum := sha256.Sum256(contents)
	return hex.EncodeToString(sum[:])
}
