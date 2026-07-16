package app

import (
	"fmt"
	"io"
	"strings"
)

type helpResult struct {
	Command string   `json:"command"`
	Usage   string   `json:"usage"`
	Purpose string   `json:"purpose"`
	Notes   []string `json:"notes,omitempty"`
}

func containsHelp(arguments []string) bool {
	for _, argument := range arguments {
		if argument == "--help" || argument == "-h" {
			return true
		}
	}
	return false
}

func runHelp(arguments []string, asJSON bool, stdout, stderr io.Writer) int {
	if len(arguments) == 0 {
		_, _ = io.WriteString(stdout, usage)
		return 0
	}
	if len(arguments) != 1 {
		return writeError(stderr, asJSON, fmt.Errorf("usage: work-ledger help <command>"))
	}

	command, found := findCommandReference(arguments[0])
	if !found || command.Name == "global" {
		return writeError(stderr, asJSON, fmt.Errorf("unknown command %q", arguments[0]))
	}
	result := helpResult{
		Command: command.Name,
		Usage:   command.Syntax,
		Purpose: command.Purpose,
		Notes:   command.Notes,
	}
	return writeResult(stdout, asJSON, result, func() {
		fmt.Fprintf(stdout, "Usage: %s\n\n%s\n", result.Usage, result.Purpose)
		if len(result.Notes) == 0 {
			return
		}
		fmt.Fprintln(stdout, "\nDetails:")
		for _, note := range result.Notes {
			fmt.Fprintf(stdout, "- %s\n", note)
		}
	})
}

func findCommandReference(name string) (primeCommand, bool) {
	for _, command := range primeCommands {
		if command.Name == name {
			return command, true
		}
	}
	return primeCommand{}, false
}

func renderCommandNotes(notes []string) string {
	return strings.Join(notes, "; ")
}
