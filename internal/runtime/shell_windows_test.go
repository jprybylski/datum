//go:build windows

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
	out, err := RunShell(context.Background(), "echo failing & exit 3", nil)
	if err == nil {
		t.Fatal("RunShell() expected error for non-zero exit, got nil")
	}
	if !strings.Contains(out, "failing") {
		t.Errorf("RunShell() output = %q, want it to contain %q", out, "failing")
	}
}

func TestRunShell_EnvAddedNotReplaced(t *testing.T) {
	// Regression test: a previous version set cmd.Env = append(cmd.Env, env...) starting from a
	// nil cmd.Env, which replaces rather than extends the process environment - wiping PATH and
	// breaking any command that isn't a cmd.exe builtin.
	out, err := RunShell(context.Background(), "echo FOO=%FOO%", []string{"FOO=bar-value"})
	if err != nil {
		t.Fatalf("RunShell() error = %v", err)
	}
	if !strings.Contains(out, "FOO=bar-value") {
		t.Errorf("RunShell() output = %q, want it to contain custom env var", out)
	}

	out, err = RunShell(context.Background(), "echo SYSTEMROOT=%SYSTEMROOT%", []string{"FOO=bar-value"})
	if err != nil {
		t.Fatalf("RunShell() error = %v", err)
	}
	if strings.Contains(out, "SYSTEMROOT=%SYSTEMROOT%") {
		t.Errorf("RunShell() output = %q, inherited environment was wiped by custom env vars", out)
	}
}

func TestRunShell_ContextCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := RunShell(ctx, "ping -n 5 127.0.0.1 > nul", nil); err == nil {
		t.Error("RunShell() expected error for already-cancelled context, got nil")
	}
}

func TestRunShell_ContextTimeout(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()
	if _, err := RunShell(ctx, "ping -n 10 127.0.0.1 > nul", nil); err == nil {
		t.Error("RunShell() expected error for timed-out context, got nil")
	}
}
