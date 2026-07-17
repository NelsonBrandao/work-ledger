package app

import (
	"strings"
	"testing"
)

func TestRenderTaskMarkdownColorModes(t *testing.T) {
	body := "# Heading\n\n- **important**\n\n```go\nfmt.Println(\"hello\")\n```\n"

	piped, err := renderTaskMarkdown(body, colorModeAuto, terminalCapabilities{})
	if err != nil {
		t.Fatalf("render piped Markdown: %v", err)
	}
	if piped != body {
		t.Fatalf("piped Markdown = %q, want original %q", piped, body)
	}

	always, err := renderTaskMarkdown(body, colorModeAlways, terminalCapabilities{width: 80})
	if err != nil {
		t.Fatalf("render forced-color Markdown: %v", err)
	}
	if !strings.Contains(always, "\x1b[") {
		t.Fatalf("forced-color Markdown has no ANSI escapes:\n%s", always)
	}
	if !strings.Contains(always, "fmt") || !strings.Contains(always, "Println") || !strings.Contains(always, "hello") {
		t.Fatalf("rendered Markdown lost code block:\n%s", always)
	}

	never, err := renderTaskMarkdown(body, colorModeNever, terminalCapabilities{width: 80})
	if err != nil {
		t.Fatalf("render no-color Markdown: %v", err)
	}
	if strings.Contains(never, "\x1b[") {
		t.Fatalf("no-color Markdown contains ANSI escapes:\n%s", never)
	}
	if !strings.Contains(never, "fmt.Println") {
		t.Fatalf("rendered no-color Markdown lost code block:\n%s", never)
	}

	noColorAuto, err := renderTaskMarkdown(body, colorModeAuto, terminalCapabilities{
		isTerminal:     true,
		width:          80,
		colorSupported: false,
	})
	if err != nil {
		t.Fatalf("render auto Markdown without terminal color: %v", err)
	}
	if strings.Contains(noColorAuto, "\x1b[") {
		t.Fatalf("auto Markdown with unavailable color contains ANSI escapes:\n%s", noColorAuto)
	}
}

func TestParseColorMode(t *testing.T) {
	t.Parallel()

	for _, value := range []string{"auto", "always", "never"} {
		if mode, err := parseColorMode(value); err != nil || string(mode) != value {
			t.Fatalf("parseColorMode(%q) = %q, %v", value, mode, err)
		}
	}
	if _, err := parseColorMode("rainbow"); err == nil || !strings.Contains(err.Error(), "--color must") {
		t.Fatalf("parseColorMode(rainbow) error = %v", err)
	}
}

func TestParseShowArguments(t *testing.T) {
	t.Parallel()

	options, err := parseShowArguments([]string{"--color", "always", "wl-123"})
	if err != nil {
		t.Fatalf("parse show arguments: %v", err)
	}
	if options.taskID != "wl-123" || options.color != colorModeAlways {
		t.Fatalf("show options = %#v", options)
	}

	for _, arguments := range [][]string{
		{"--color"},
		{"--color", "rainbow", "wl-123"},
		{"--unknown", "wl-123"},
		{"wl-one", "wl-two"},
	} {
		if _, err := parseShowArguments(arguments); err == nil {
			t.Fatalf("parseShowArguments(%v) succeeded", arguments)
		}
	}
}

func TestTerminalSupportsColorHonorsEnvironment(t *testing.T) {
	t.Setenv("NO_COLOR", "1")
	if terminalSupportsColor() {
		t.Fatal("terminalSupportsColor ignored NO_COLOR")
	}

	t.Setenv("NO_COLOR", "")
	t.Setenv("TERM", "dumb")
	if terminalSupportsColor() {
		t.Fatal("terminalSupportsColor ignored TERM=dumb")
	}
}
