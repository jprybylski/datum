package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jprybylski/datum/internal/core"
)

func TestRun_VersionText_IsUnchanged(t *testing.T) {
	old := version
	version = "v1.2.3"
	defer func() { version = old }()
	out, code := captureRun(t, []string{"--version"})
	if code != 0 || out != "datum v1.2.3\n" {
		t.Errorf("--version = %d %q; wrappers parse exactly this line", code, out)
	}
}

func TestRun_VersionJSON(t *testing.T) {
	old := version
	version = "v1.2.3"
	defer func() { version = old; core.JSONOutput = false }()
	out, code := captureRun(t, []string{"--json", "--version"})
	var doc struct {
		Format   string   `json:"format"`
		Version  string   `json:"version"`
		Features []string `json:"features"`
	}
	if err := json.Unmarshal([]byte(out), &doc); err != nil || code != 0 {
		t.Fatalf("--json --version = %d %q (%v)", code, out, err)
	}
	if doc.Format != core.ReportFormat || doc.Version != "1.2.3" || doc.Features == nil {
		t.Errorf("--json --version = %+v", doc)
	}
}

// The managed-mode flags reach the core: one run with all of them, on a config that breaks
// several, must fail on the first rule it checks.
func TestRun_ManagedFlags(t *testing.T) {
	dir := t.TempDir()
	cfg := filepath.Join(dir, ".data.yaml")
	if err := os.WriteFile(cfg, []byte(`version: 1
datasets:
  - id: shell
    desc: x
    source: {type: command, fingerprint_cmd: "echo 1", fetch_cmd: "echo 1"}
    target: data/x
`), 0o644); err != nil {
		t.Fatal(err)
	}
	lock := filepath.Join(dir, ".data.lock.yaml")
	base := []string{"--config", cfg, "--lock", lock}

	out, code := captureRun(t, append(base, "--allow-types", "http, file", "check"))
	if code != 2 || !strings.Contains(out, `"command" is not allowed`) {
		t.Errorf("--allow-types: %d %q", code, out)
	}

	t.Setenv("DATUM_ALLOW_TYPES", "http,file")
	out, code = captureRun(t, append(base, "check"))
	if code != 2 || !strings.Contains(out, "not allowed") {
		t.Errorf("DATUM_ALLOW_TYPES: %d %q", code, out)
	}
	out, code = captureRun(t, append(base, "--allow-types", "command", "--report-only", "--no-vcs", "check"))
	if code != 1 || !strings.Contains(out, "[FAIL] shell") { // never fetched, fail policy
		t.Errorf("--allow-types overrides DATUM_ALLOW_TYPES: %d %q", code, out)
	}
	if _, err := os.Stat(lock); err == nil {
		t.Error("--report-only wrote a lockfile")
	}

	out, code = captureRun(t, append(base, "--root", dir, "--target-prefix", "source-data", "--allow-types", "command", "audit"))
	if code != 2 || !strings.Contains(out, "--target-prefix") {
		t.Errorf("--target-prefix: %d %q", code, out)
	}
	out, code = captureRun(t, append(base, "--stage-dir", filepath.Join(dir, "s"), "--allow-types", "command", "fetch"))
	if code != 2 || !strings.Contains(out, "--lock-out") {
		t.Errorf("--stage-dir alone: %d %q", code, out)
	}
	out, code = captureRun(t, append(base, "--env-allow", "A,B", "--env-prefix", "P_", "--https-only", "--git-https-only",
		"--cache-dir", t.TempDir(), "--verify-local", "--allow-types", "command", "audit"))
	if code != 0 {
		t.Errorf("all flags parse: %d %q", code, out)
	}
}

func TestRun_InitJSONNeverPrompts(t *testing.T) {
	defer func() { core.JSONOutput = false }()
	cfg := filepath.Join(t.TempDir(), ".data.yaml")
	out, code := captureRun(t, []string{"--json", "--no-vcs", "--config", cfg, "init", "--empty"})
	var doc map[string]any
	if err := json.Unmarshal([]byte(out), &doc); err != nil || code != 0 {
		t.Fatalf("init --json = %d %q", code, out)
	}
	if doc["config"] != cfg || len(doc["datasets"].([]any)) != 0 {
		t.Errorf("init --json = %v", doc)
	}
}
