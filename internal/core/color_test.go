package core

import "testing"

// colorEnabled's terminal-detection branch can't easily be forced true in a unit test (there's no
// real TTY under `go test`), so these tests focus on the "must stay off" paths - which is exactly
// the behavior that matters for CI logs, piped output, and NO_COLOR compliance.

func TestColorizeDisabledByNoColorVar(t *testing.T) {
	NoColor = true
	defer func() { NoColor = false }()

	if got := colorize(ansiGreen, "[OK  ]"); got != "[OK  ]" {
		t.Errorf("colorize() with NoColor=true = %q, want unchanged %q", got, "[OK  ]")
	}
}

func TestColorizeDisabledByNoColorEnv(t *testing.T) {
	t.Setenv("NO_COLOR", "1")

	if got := colorize(ansiRed, "[ERR ]"); got != "[ERR ]" {
		t.Errorf("colorize() with NO_COLOR set = %q, want unchanged %q", got, "[ERR ]")
	}
}

func TestColorizeDisabledUnderTest(t *testing.T) {
	// go test's stdout isn't a terminal, so colorEnabled() should report false even with no
	// override set, and colorize should pass its input through untouched.
	if colorEnabled() {
		t.Fatal("colorEnabled() = true under go test, want false (stdout isn't a terminal)")
	}
	if got := colorize(ansiYellow, "[WARN]"); got != "[WARN]" {
		t.Errorf("colorize() = %q, want unchanged %q", got, "[WARN]")
	}
}
