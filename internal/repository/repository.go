package repository

import (
	"fmt"
	"net/url"
	"os/exec"
	"strings"
)

type Context struct {
	Root   string `json:"root"`
	Remote string `json:"remote"`
	ID     string `json:"id"`
}

func Discover(directory string) (Context, error) {
	root, err := runGit(directory, "rev-parse", "--show-toplevel")
	if err != nil {
		return Context{}, fmt.Errorf("discover Git repository: %w", err)
	}
	remote, err := runGit(directory, "remote", "get-url", "origin")
	if err != nil {
		return Context{}, fmt.Errorf("read origin remote: %w", err)
	}
	id, err := NormalizeRemote(remote)
	if err != nil {
		return Context{}, err
	}
	return Context{Root: root, Remote: remote, ID: id}, nil
}

func ResolveAlias(id string, aliases map[string][]string) string {
	for canonical, values := range aliases {
		if id == canonical {
			return canonical
		}
		for _, alias := range values {
			if id == alias {
				return canonical
			}
		}
	}
	return id
}

func NormalizeRemote(remote string) (string, error) {
	trimmed := strings.TrimSpace(remote)
	if trimmed == "" {
		return "", fmt.Errorf("Git remote is empty")
	}

	if strings.Contains(trimmed, "://") {
		parsed, err := url.Parse(trimmed)
		if err != nil || parsed.Hostname() == "" {
			return "", fmt.Errorf("unsupported Git remote %q", remote)
		}
		return normalizeParts(parsed.Hostname(), parsed.Path, remote)
	}

	if at := strings.LastIndex(trimmed, "@"); at >= 0 {
		remainder := trimmed[at+1:]
		if colon := strings.Index(remainder, ":"); colon > 0 {
			return normalizeParts(remainder[:colon], remainder[colon+1:], remote)
		}
	}

	return "", fmt.Errorf("unsupported Git remote %q", remote)
}

func normalizeParts(host, remotePath, original string) (string, error) {
	path := strings.Trim(strings.TrimSuffix(remotePath, ".git"), "/")
	if host == "" || path == "" {
		return "", fmt.Errorf("unsupported Git remote %q", original)
	}
	return strings.ToLower(host) + "/" + path, nil
}

func runGit(directory string, arguments ...string) (string, error) {
	command := exec.Command("git", append([]string{"-C", directory}, arguments...)...)
	output, err := command.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("git %s: %s", strings.Join(arguments, " "), strings.TrimSpace(string(output)))
	}
	return strings.TrimSpace(string(output)), nil
}
