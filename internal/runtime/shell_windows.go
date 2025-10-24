//go:build windows

// Package runtime provides platform-specific shell command execution.
//
// This file (shell_windows.go) is compiled only on Windows due to the build constraint above.
//
// Windows shell handling is more complex than Unix because Windows has multiple shell options:
//   - PowerShell (preferred, more powerful and modern)
//   - cmd.exe (fallback for older systems or when PowerShell is not available)
package runtime

import (
    "context"
    "fmt"
    "os/exec"
)

// RunShell executes a shell command using PowerShell or cmd.exe on Windows.
//
// The function prefers PowerShell if available (more powerful and modern), but falls back
// to cmd.exe if PowerShell is not found in the system PATH.
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
// PowerShell flags explained:
//   - -NoProfile: Don't load user profile (faster startup)
//   - -ExecutionPolicy Bypass: Allow script execution without policy restrictions
//   - -Command: Execute the following command string
func RunShell(ctx context.Context, cmdline string, env []string) (string, error) {
    var cmd *exec.Cmd

    // Try to find PowerShell in the system PATH
    if _, err := exec.LookPath("powershell"); err == nil {
        // PowerShell is available - use it with flags to bypass profile and execution policy
        cmd = exec.CommandContext(ctx, "powershell", "-NoProfile", "-ExecutionPolicy", "Bypass", "-Command", cmdline)
    } else {
        // PowerShell not found - fall back to cmd.exe
        // /C means "execute command and then terminate"
        cmd = exec.CommandContext(ctx, "cmd", "/C", cmdline)
    }

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
