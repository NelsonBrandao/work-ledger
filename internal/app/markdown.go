package app

import (
	"fmt"
	"io"
	"os"

	"github.com/charmbracelet/glamour"
	"github.com/charmbracelet/glamour/styles"
	"github.com/muesli/termenv"
	"golang.org/x/term"
)

const defaultMarkdownWidth = 80

type colorMode string

const (
	colorModeAuto   colorMode = "auto"
	colorModeAlways colorMode = "always"
	colorModeNever  colorMode = "never"
)

type terminalCapabilities struct {
	isTerminal     bool
	width          int
	colorSupported bool
}

func terminalCapabilitiesFor(writer io.Writer) terminalCapabilities {
	capabilities := terminalCapabilities{
		width:          defaultMarkdownWidth,
		colorSupported: terminalSupportsColor(),
	}
	file, ok := writer.(*os.File)
	if !ok || !term.IsTerminal(int(file.Fd())) {
		return capabilities
	}

	capabilities.isTerminal = true
	if width, _, err := term.GetSize(int(file.Fd())); err == nil && width >= 40 {
		capabilities.width = width
	}
	return capabilities
}

func terminalSupportsColor() bool {
	if os.Getenv("NO_COLOR") != "" {
		return false
	}
	return os.Getenv("TERM") != "dumb"
}

func parseColorMode(value string) (colorMode, error) {
	mode := colorMode(value)
	switch mode {
	case colorModeAuto, colorModeAlways, colorModeNever:
		return mode, nil
	default:
		return "", fmt.Errorf("--color must be auto, always, or never; got %q", value)
	}
}

func renderTaskMarkdown(body string, mode colorMode, terminal terminalCapabilities) (string, error) {
	if mode == colorModeAuto && !terminal.isTerminal {
		return body, nil
	}

	width := terminal.width
	if width < 40 {
		width = defaultMarkdownWidth
	}
	options := []glamour.TermRendererOption{
		glamour.WithWordWrap(width),
	}
	switch {
	case mode == colorModeAlways:
		options = append(options,
			glamour.WithStandardStyle(styles.DarkStyle),
			glamour.WithColorProfile(termenv.TrueColor),
		)
	case mode == colorModeNever || !terminal.colorSupported:
		options = append(options,
			glamour.WithStandardStyle(styles.NoTTYStyle),
			glamour.WithColorProfile(termenv.Ascii),
		)
	default:
		options = append(options,
			glamour.WithAutoStyle(),
			glamour.WithColorProfile(termenv.TrueColor),
		)
	}

	renderer, err := glamour.NewTermRenderer(options...)
	if err != nil {
		return "", err
	}
	return renderer.Render(body)
}
