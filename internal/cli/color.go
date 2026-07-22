// Package cli holds small presentation helpers shared by the CLI
// commands: ANSI coloring and a minimal progress indicator.
//
// NOTE ON DEPENDENCIES: the spec suggests fatih/color, AlecAivazis/survey,
// and schollz/progressbar for these jobs. This dev sandbox's network
// egress allowlist doesn't include gopkg.in, which cobra (and much of
// that CLI ecosystem) pulls in transitively, so this package hand-rolls
// the thin slice of functionality actually used (a handful of ANSI SGR
// codes, a line-overwriting progress counter, and basic stdin prompts) on
// the stdlib alone. If your real network allows it, swapping in those
// libraries is a drop-in replacement — the call sites in cmd/alloyctl are
// small and isolated.
package cli

import (
	"fmt"
	"os"
)

const (
	reset  = "\x1b[0m"
	green  = "\x1b[1;32m"   // bold green
	yellow = "\x1b[1;33m"   // bold yellow
	gray   = "\x1b[2m"      // dim
	red    = "\x1b[1;31m"   // bold red
	cyan   = "\x1b[1;36m"   // bold cyan
)

var colorEnabled = os.Getenv("NO_COLOR") == ""

func paint(code, s string) string {
	if !colorEnabled {
		return s
	}
	return code + s + reset
}

// ColorForVersionType returns the ANSI-wrapped string for a Mojang
// version "type" field, matching the install/play listing rules:
// release -> green, snapshot -> yellow, old_beta/old_alpha -> gray.
func ColorForVersionType(versionType, text string) string {
	switch versionType {
	case "release":
		return paint(green, text)
	case "snapshot":
		return paint(yellow, text)
	case "old_beta", "old_alpha":
		return paint(gray, text)
	default:
		return text
	}
}

func Errorf(format string, a ...any) {
	fmt.Fprintln(os.Stderr, paint(red, "error: "+fmt.Sprintf(format, a...)))
}

func Info(s string) {
	fmt.Println(paint(cyan, s))
}
