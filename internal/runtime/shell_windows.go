//go:build windows

// Package runtime provides platform-specific shell command execution.
//
// This file (shell_windows.go) is compiled only on Windows due to the build constraint above.
//
// Windows shell handling: We use cmd.exe instead of PowerShell for better compatibility
// and to avoid PowerShell's UTF-16 LE default encoding for file redirects (the > operator).
// PowerShell 5.x uses UTF-16 LE by default which causes issues with cross-platform tests
// that expect UTF-8. cmd.exe uses the system code page which is more predictable.
package runtime

import (
	"context"
	"fmt"
	"os/exec"
)

// RunShell executes a shell command using cmd.exe on Windows.
//
// We use cmd.exe rather than PowerShell to avoid encoding issues with file redirection.
// PowerShell 5.x (still common on Windows) uses UTF-16 LE encoding by default for the
// > redirect operator, which causes cross-platform compatibility issues.
//
// Parameters:
//   - ctx: Context for cancellation and timeout control
//   - cmdline: The complete shell command to execute
//   - env: Optional environment variables in "KEY=value" format (can be nil)
//
// Returns:
//   - The command's combined stdout and stderr output
//   - An error if the command fails or returns non-zero exit code
//
// cmd.exe flags explained:
//   - /C: Execute the command and then terminate
func RunShell(ctx context.Context, cmdline string, env []string) (string, error) {
	// Use cmd.exe for consistent cross-platform behavior
	// /C means "execute command and then terminate"
	cmd := exec.CommandContext(ctx, "cmd", "/C", cmdline)

	// Append custom environment variables if provided
	if env != nil {
		cmd.Env = append(cmd.Env, env...)
	}

	// CombinedOutput runs the command and captures both stdout and stderr
	out, err := cmd.CombinedOutput()
	if err != nil {
		// Include both the error and the output for better debugging
		return string(out), fmt.Errorf("command failed: %s\n%s", err, string(out))
	}
	return string(out), nil
}
