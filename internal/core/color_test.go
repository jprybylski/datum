package core

import "testing"

// withFakeTerminal overrides isTerminal to report a fake TTY for the duration of fn, restoring it
// afterward - the only practical way to exercise the "colors on" path, since there's no real TTY
// under `go test`.
func withFakeTerminal(t *testing.T, fn func()) {
	t.Helper()
	orig := isTerminal
	isTerminal = func() bool { return true }
	defer func() { isTerminal = orig }()
	fn()
}

func TestColorizeEnabledWithFakeTerminal(t *testing.T) {
	withFakeTerminal(t, func() {
		if !colorEnabled() {
			t.Fatal("colorEnabled() = false with a fake terminal, want true")
		}
		got := colorize(ansiGreen, "[OK  ]")
		want := "\x1b[32m[OK  ]\x1b[0m"
		if got != want {
			t.Errorf("colorize() = %q, want %q", got, want)
		}
	})
}

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
