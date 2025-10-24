//go:build !windows

// Package runtime provides platform-specific shell command execution.
//
// This file (shell_unix.go) is compiled on all non-Windows platforms (Linux, macOS, BSD, etc.)
// due to the build constraint "!windows" above.
//
// Go beginners: Build tags enable conditional compilation based on platform, architecture,
// or custom flags. This allows different implementations for different operating systems
// while maintaining the same package interface.
package runtime

import (
    "context"
    "fmt"
    "os/exec"
)

// RunShell executes a shell command using /bin/sh on Unix-like systems.
//
// Parameters:
//   - ctx: Context for cancellation and timeout control
//   - cmdline: The complete shell command to execute (passed to "sh -c")
//   - env: Optional environment variables in "KEY=value" format (can be nil)
//
// Returns:
//   - The command's combined stdout and stderr output
//   - An error if the command fails or returns non-zero exit code
//
// Security note: cmdline is executed in a shell, so be careful with user input.
// The command runs with the same permissions as the datum process.
func RunShell(ctx context.Context, cmdline string, env []string) (string, error) {
    // CommandContext creates a command that will be killed if ctx is cancelled
    cmd := exec.CommandContext(ctx, "sh", "-c", cmdline)

    // Append custom environment variables if provided
    // Note: This adds to the existing environment, not replaces it
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
