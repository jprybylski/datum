package core

import (
	"os"

	"golang.org/x/term"
)

// NoColor forces colorize to always return its input unchanged, regardless of whether stdout is
// a terminal. Set by the CLI's --no-color flag before calling Check/Fetch.
var NoColor bool

// ANSI SGR (Select Graphic Rendition) color codes used for status tags below.
const (
	ansiGreen  = "32"
	ansiRed    = "31"
	ansiYellow = "33"
	ansiCyan   = "36"
	ansiBlue   = "34"
	ansiDim    = "2"
)

// colorEnabled reports whether output written to stdout should include ANSI color codes.
//
// Go learning note: term.IsTerminal takes a raw file descriptor (an int), not an *os.File -
// that's why we pass os.Stdout.Fd() rather than os.Stdout itself. It returns false for pipes,
// redirected files, and (importantly for tests) the os.Pipe() writers test code substitutes for
// os.Stdout, so color is automatically suppressed under `go test` and when output is piped.
func colorEnabled() bool {
	if NoColor || os.Getenv("NO_COLOR") != "" {
		return false
	}
	return term.IsTerminal(int(os.Stdout.Fd()))
}

// colorize wraps s in the given ANSI color code when colorEnabled, otherwise returns s unchanged.
// It's used to color just the bracketed status tag (e.g. "[OK  ]") in progress lines, leaving the
// surrounding message text - and any test assertions matching the literal tag substring - alone.
func colorize(ansiCode, s string) string {
	if !colorEnabled() {
		return s
	}
	return "\x1b[" + ansiCode + "m" + s + "\x1b[0m"
}
