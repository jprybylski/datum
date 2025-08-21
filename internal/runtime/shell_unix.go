//go:build !windows

package runtime

import (
    "context"
    "fmt"
    "os/exec"
)

func RunShell(ctx context.Context, cmdline string, env []string) (string, error) {
    cmd := exec.CommandContext(ctx, "sh", "-c", cmdline)
    if env != nil { cmd.Env = append(cmd.Env, env...) }
    out, err := cmd.CombinedOutput()
    if err != nil {
        return string(out), fmt.Errorf("command failed: %s\n%s", err, string(out))
    }
    return string(out), nil
}
