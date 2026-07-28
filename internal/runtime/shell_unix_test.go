//go:build !windows

package runtime

import (
	"context"
	"strings"
	"testing"
	"time"
)

func TestRunShell_Success(t *testing.T) {
	out, err := RunShell(context.Background(), "echo hello", nil)
	if err != nil {
		t.Fatalf("RunShell() error = %v", err)
	}
	if !strings.Contains(out, "hello") {
		t.Errorf("RunShell() output = %q, want it to contain %q", out, "hello")
	}
}

func TestRunShell_NonZeroExit(t *testing.T) {
	out, err := RunShell(context.Background(), "echo failing; exit 3", nil)
	if err == nil {
		t.Fatal("RunShell() expected error for non-zero exit, got nil")
	}
	if !strings.Contains(out, "failing") {
		t.Errorf("RunShell() output = %q, want it to contain %q", out, "failing")
	}
	if !strings.Contains(err.Error(), "failing") {
		t.Errorf("RunShell() error = %v, want it to include command output", err)
	}
}

func TestRunShell_EnvAddedNotReplaced(t *testing.T) {
	// Regression test: a previous version set cmd.Env = append(cmd.Env, env...) starting from a
	// nil cmd.Env, which replaces rather than extends the process environment - wiping PATH and
	// breaking any command that isn't a shell builtin. Confirm both the custom var arrives AND
	// the inherited PATH is still usable (via a builtin that depends on shell setup, "true").
	out, err := RunShell(context.Background(), "echo FOO=$FOO; true", []string{"FOO=bar-value"})
	if err != nil {
		t.Fatalf("RunShell() error = %v", err)
	}
	if !strings.Contains(out, "FOO=bar-value") {
		t.Errorf("RunShell() output = %q, want it to contain custom env var", out)
	}

	out, err = RunShell(context.Background(), "echo PATH_LEN=${#PATH}", []string{"FOO=bar-value"})
	if err != nil {
		t.Fatalf("RunShell() error = %v", err)
	}
	if strings.Contains(out, "PATH_LEN=0") {
		t.Errorf("RunShell() output = %q, PATH was wiped by custom env vars", out)
	}
}

func TestRunShell_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := RunShell(ctx, "sleep 1", nil); err == nil {
		t.Error("RunShell() expected error for already-cancelled context, got nil")
	}
}

func TestRunShell_ContextTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := RunShell(ctx, "sleep 5", nil); err == nil {
		t.Error("RunShell() expected error for timed-out context, got nil")
	}
}
