package main

import (
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	datumlib "github.com/jprybylski/datum"
	"github.com/jprybylski/datum/internal/core"
)

func TestRun(t *testing.T) {
	t.Run("no args shows usage and exits 2", func(t *testing.T) {
		if code := run(nil); code != 2 {
			t.Errorf("run(nil) = %d, want 2", code)
		}
	})

	t.Run("unknown subcommand exits 2", func(t *testing.T) {
		if code := run([]string{"bogus"}); code != 2 {
			t.Errorf("run([bogus]) = %d, want 2", code)
		}
	})

	t.Run("--version exits 0 without requiring a subcommand", func(t *testing.T) {
		if code := run([]string{"--version"}); code != 0 {
			t.Errorf("run([--version]) = %d, want 0", code)
		}
	})

	t.Run("unknown flag exits 2", func(t *testing.T) {
		if code := run([]string{"--not-a-real-flag", "check"}); code != 2 {
			t.Errorf("run([--not-a-real-flag check]) = %d, want 2", code)
		}
	})

	t.Run("check with missing config exits 2", func(t *testing.T) {
		tmpDir := t.TempDir()
		code := run([]string{
			"--config", filepath.Join(tmpDir, "missing.yaml"),
			"--lock", filepath.Join(tmpDir, "lock.yaml"),
			"check",
		})
		if code != 2 {
			t.Errorf("run(check with missing config) = %d, want 2", code)
		}
	})

	t.Run("fetch dispatches and fetches a file source", func(t *testing.T) {
		tmpDir := t.TempDir()
		srcFile := filepath.Join(tmpDir, "src.txt")
		if err := os.WriteFile(srcFile, []byte("hello"), 0o644); err != nil {
			t.Fatal(err)
		}
		targetFile := filepath.Join(tmpDir, "target.txt")
		cfgPath := filepath.Join(tmpDir, ".data.yaml")
		cfgContent := "version: 1\ndatasets:\n  - id: main_test\n    source:\n      type: file\n      path: " + srcFile + "\n    target: " + targetFile + "\n"
		if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o644); err != nil {
			t.Fatal(err)
		}
		lockPath := filepath.Join(tmpDir, ".data.lock.yaml")

		code := run([]string{"--config", cfgPath, "--lock", lockPath, "fetch"})
		if code != 0 {
			t.Errorf("run(fetch) = %d, want 0", code)
		}
		if _, err := os.Stat(targetFile); err != nil {
			t.Errorf("target file not created: %v", err)
		}

		// check should now succeed against the freshly fetched target
		code = run([]string{"--config", cfgPath, "--lock", lockPath, "check"})
		if code != 0 {
			t.Errorf("run(check) after fetch = %d, want 0", code)
		}
	})

	t.Run("fetch with explicit ids", func(t *testing.T) {
		tmpDir := t.TempDir()
		srcFile := filepath.Join(tmpDir, "src.txt")
		if err := os.WriteFile(srcFile, []byte("hello"), 0o644); err != nil {
			t.Fatal(err)
		}
		targetFile := filepath.Join(tmpDir, "target.txt")
		cfgPath := filepath.Join(tmpDir, ".data.yaml")
		cfgContent := "version: 1\ndatasets:\n  - id: only_one\n    source:\n      type: file\n      path: " + srcFile + "\n    target: " + targetFile + "\n"
		if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o644); err != nil {
			t.Fatal(err)
		}
		lockPath := filepath.Join(tmpDir, ".data.lock.yaml")

		code := run([]string{"--config", cfgPath, "--lock", lockPath, "fetch", "only_one"})
		if code != 0 {
			t.Errorf("run(fetch only_one) = %d, want 0", code)
		}
	})

	t.Run("--no-color and --json set the corresponding core package vars", func(t *testing.T) {
		defer func() {
			core.NoColor = false
			core.JSONOutput = false
		}()

		tmpDir := t.TempDir()
		srcFile := filepath.Join(tmpDir, "src.txt")
		if err := os.WriteFile(srcFile, []byte("hello"), 0o644); err != nil {
			t.Fatal(err)
		}
		targetFile := filepath.Join(tmpDir, "target.txt")
		cfgPath := filepath.Join(tmpDir, ".data.yaml")
		cfgContent := "version: 1\ndatasets:\n  - id: main_test\n    source:\n      type: file\n      path: " + srcFile + "\n    target: " + targetFile + "\n"
		if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o644); err != nil {
			t.Fatal(err)
		}
		lockPath := filepath.Join(tmpDir, ".data.lock.yaml")

		code := run([]string{"--no-color", "--json", "--config", cfgPath, "--lock", lockPath, "fetch"})
		if code != 0 {
			t.Errorf("run(fetch with --no-color --json) = %d, want 0", code)
		}
		if !core.NoColor {
			t.Error("core.NoColor = false, want true after --no-color")
		}
		if !core.JSONOutput {
			t.Error("core.JSONOutput = false, want true after --json")
		}
	})

	t.Run("invalid timeout value exits 2", func(t *testing.T) {
		code := run([]string{"--timeout", "not-a-duration", "check"})
		if code != 2 {
			t.Errorf("run(--timeout not-a-duration) = %d, want 2", code)
		}
	})

	t.Run("negative concurrency is clamped to 1, not a crash or hang", func(t *testing.T) {
		tmpDir := t.TempDir()
		srcFile := filepath.Join(tmpDir, "src.txt")
		if err := os.WriteFile(srcFile, []byte("hello"), 0o644); err != nil {
			t.Fatal(err)
		}
		targetFile := filepath.Join(tmpDir, "target.txt")
		cfgPath := filepath.Join(tmpDir, ".data.yaml")
		cfgContent := "version: 1\ndatasets:\n  - id: neg_concurrency\n    source:\n      type: file\n      path: " + srcFile + "\n    target: " + targetFile + "\n"
		if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o644); err != nil {
			t.Fatal(err)
		}
		lockPath := filepath.Join(tmpDir, ".data.lock.yaml")

		code := run([]string{"--config", cfgPath, "--lock", lockPath, "--concurrency", "-3", "fetch"})
		if code != 0 {
			t.Errorf("run(--concurrency -3) = %d, want 0", code)
		}
	})

	t.Run("concurrency and timeout flags are accepted and datasets still fetch", func(t *testing.T) {
		tmpDir := t.TempDir()
		cfgPath := filepath.Join(tmpDir, ".data.yaml")
		var cfgContent string
		cfgContent += "version: 1\ndatasets:\n"
		for i := 0; i < 4; i++ {
			id := "ds" + string(rune('a'+i))
			srcFile := filepath.Join(tmpDir, id+"-src.txt")
			if err := os.WriteFile(srcFile, []byte(id), 0o644); err != nil {
				t.Fatal(err)
			}
			targetFile := filepath.Join(tmpDir, id+"-target.txt")
			cfgContent += "  - id: " + id + "\n    source:\n      type: file\n      path: " + srcFile + "\n    target: " + targetFile + "\n"
		}
		if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o644); err != nil {
			t.Fatal(err)
		}
		lockPath := filepath.Join(tmpDir, ".data.lock.yaml")

		code := run([]string{
			"--config", cfgPath, "--lock", lockPath,
			"--timeout", "30s", "--concurrency", "4",
			"fetch",
		})
		if code != 0 {
			t.Errorf("run(--concurrency 4) = %d, want 0", code)
		}
		for i := 0; i < 4; i++ {
			id := "ds" + string(rune('a'+i))
			if _, err := os.Stat(filepath.Join(tmpDir, id+"-target.txt")); err != nil {
				t.Errorf("target for %s not created: %v", id, err)
			}
		}
	})
}

func TestRunInit(t *testing.T) {
	dir := t.TempDir()
	configPath := filepath.Join(dir, "generated.yaml")
	code := run([]string{
		"--config", configPath, "init",
		"--id", "starter", "--type", "file", "--source", "source.csv",
		"--target", "data/starter.csv", "--desc", "Starter data", "--policy", "log",
	})
	if code != 0 {
		t.Fatalf("run(init) = %d", code)
	}
	content, err := os.ReadFile(configPath)
	if err != nil {
		t.Fatal(err)
	}
	for _, want := range []string{"id: starter", "type: file", "path: source.csv", "policy: log"} {
		if !strings.Contains(string(content), want) {
			t.Errorf("generated config missing %q:\n%s", want, content)
		}
	}
	if code := run([]string{"--config", filepath.Join(dir, "missing.yaml"), "init", "--id", "incomplete"}); code != 2 {
		t.Errorf("run(incomplete init) = %d, want 2", code)
	}
	emptyPath := filepath.Join(dir, "empty.yaml")
	if code := run([]string{"--config", emptyPath, "init", "--empty"}); code != 0 {
		t.Fatalf("run(init --empty) = %d", code)
	}
	if content, err := os.ReadFile(emptyPath); err != nil || string(content) != "version: 1\ndatasets: []\n" {
		t.Fatalf("empty init content = %q, %v", content, err)
	}
}

func TestRun_DeleteUndelete(t *testing.T) {
	t.Run("delete with --yes removes target and marks lockfile deleted", func(t *testing.T) {
		tmpDir := t.TempDir()
		srcFile := filepath.Join(tmpDir, "src.txt")
		if err := os.WriteFile(srcFile, []byte("hello"), 0o644); err != nil {
			t.Fatal(err)
		}
		targetFile := filepath.Join(tmpDir, "target.txt")
		cfgPath := filepath.Join(tmpDir, ".data.yaml")
		cfgContent := "version: 1\ndatasets:\n  - id: del_test\n    source:\n      type: file\n      path: " + srcFile + "\n    target: " + targetFile + "\n"
		if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o644); err != nil {
			t.Fatal(err)
		}
		lockPath := filepath.Join(tmpDir, ".data.lock.yaml")

		if code := run([]string{"--config", cfgPath, "--lock", lockPath, "fetch"}); code != 0 {
			t.Fatalf("run(fetch) = %d, want 0", code)
		}
		if _, err := os.Stat(targetFile); err != nil {
			t.Fatalf("target file not created by fetch: %v", err)
		}

		code := run([]string{"--config", cfgPath, "--lock", lockPath, "--yes", "delete", "del_test"})
		if code != 0 {
			t.Errorf("run(delete --yes) = %d, want 0", code)
		}
		if _, err := os.Stat(targetFile); !os.IsNotExist(err) {
			t.Errorf("target file should have been removed by delete, stat err = %v", err)
		}

		// check must skip the deleted dataset rather than failing on the now-missing target.
		code = run([]string{"--config", cfgPath, "--lock", lockPath, "check"})
		if code != 0 {
			t.Errorf("run(check) after delete = %d, want 0 (skip, not fail)", code)
		}
	})

	t.Run("undelete clears the flag so fetch restores the data", func(t *testing.T) {
		tmpDir := t.TempDir()
		srcFile := filepath.Join(tmpDir, "src.txt")
		if err := os.WriteFile(srcFile, []byte("hello"), 0o644); err != nil {
			t.Fatal(err)
		}
		targetFile := filepath.Join(tmpDir, "target.txt")
		cfgPath := filepath.Join(tmpDir, ".data.yaml")
		cfgContent := "version: 1\ndatasets:\n  - id: undel_test\n    source:\n      type: file\n      path: " + srcFile + "\n    target: " + targetFile + "\n"
		if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o644); err != nil {
			t.Fatal(err)
		}
		lockPath := filepath.Join(tmpDir, ".data.lock.yaml")

		if code := run([]string{"--config", cfgPath, "--lock", lockPath, "fetch"}); code != 0 {
			t.Fatalf("run(fetch) = %d, want 0", code)
		}
		if code := run([]string{"--config", cfgPath, "--lock", lockPath, "--yes", "delete", "undel_test"}); code != 0 {
			t.Fatalf("run(delete --yes) = %d, want 0", code)
		}

		if code := run([]string{"--lock", lockPath, "undelete", "undel_test"}); code != 0 {
			t.Errorf("run(undelete) = %d, want 0", code)
		}

		if code := run([]string{"--config", cfgPath, "--lock", lockPath, "fetch", "undel_test"}); code != 0 {
			t.Errorf("run(fetch) after undelete = %d, want 0", code)
		}
		if _, err := os.Stat(targetFile); err != nil {
			t.Errorf("target file should exist again after undelete + fetch: %v", err)
		}
	})

	t.Run("delete with no ids shows usage and exits 2", func(t *testing.T) {
		tmpDir := t.TempDir()
		code := run([]string{"--config", filepath.Join(tmpDir, ".data.yaml"), "--lock", filepath.Join(tmpDir, ".data.lock.yaml"), "delete"})
		if code != 2 {
			t.Errorf("run(delete, no ids) = %d, want 2", code)
		}
	})

	t.Run("undelete with no ids shows usage and exits 2", func(t *testing.T) {
		tmpDir := t.TempDir()
		code := run([]string{"--lock", filepath.Join(tmpDir, ".data.lock.yaml"), "undelete"})
		if code != 2 {
			t.Errorf("run(undelete, no ids) = %d, want 2", code)
		}
	})
}

func TestRun_UnlockAudit(t *testing.T) {
	t.Run("unlock with --yes removes an orphaned lockfile entry", func(t *testing.T) {
		tmpDir := t.TempDir()
		srcFile := filepath.Join(tmpDir, "src.txt")
		if err := os.WriteFile(srcFile, []byte("hello"), 0o644); err != nil {
			t.Fatal(err)
		}
		targetFile := filepath.Join(tmpDir, "target.txt")
		cfgPath := filepath.Join(tmpDir, ".data.yaml")
		cfgContent := "version: 1\ndatasets:\n  - id: unlock_test\n    source:\n      type: file\n      path: " + srcFile + "\n    target: " + targetFile + "\n"
		if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o644); err != nil {
			t.Fatal(err)
		}
		lockPath := filepath.Join(tmpDir, ".data.lock.yaml")

		if code := run([]string{"--config", cfgPath, "--lock", lockPath, "fetch"}); code != 0 {
			t.Fatalf("run(fetch) = %d, want 0", code)
		}

		// Remove the dataset from the config so unlock has an orphaned entry to work on.
		if err := os.WriteFile(cfgPath, []byte("version: 1\ndatasets: []\n"), 0o644); err != nil {
			t.Fatal(err)
		}

		code := run([]string{"--config", cfgPath, "--lock", lockPath, "--yes", "unlock", "unlock_test"})
		if code != 0 {
			t.Errorf("run(unlock --yes) = %d, want 0", code)
		}
	})

	t.Run("unlock with no ids shows usage and exits 2", func(t *testing.T) {
		tmpDir := t.TempDir()
		code := run([]string{"--config", filepath.Join(tmpDir, ".data.yaml"), "--lock", filepath.Join(tmpDir, ".data.lock.yaml"), "unlock"})
		if code != 2 {
			t.Errorf("run(unlock, no ids) = %d, want 2", code)
		}
	})

	t.Run("audit reports a freshly fetched dataset as ok", func(t *testing.T) {
		tmpDir := t.TempDir()
		srcFile := filepath.Join(tmpDir, "src.txt")
		if err := os.WriteFile(srcFile, []byte("hello"), 0o644); err != nil {
			t.Fatal(err)
		}
		targetFile := filepath.Join(tmpDir, "target.txt")
		cfgPath := filepath.Join(tmpDir, ".data.yaml")
		cfgContent := "version: 1\ndatasets:\n  - id: audit_test\n    source:\n      type: file\n      path: " + srcFile + "\n    target: " + targetFile + "\n"
		if err := os.WriteFile(cfgPath, []byte(cfgContent), 0o644); err != nil {
			t.Fatal(err)
		}
		lockPath := filepath.Join(tmpDir, ".data.lock.yaml")

		if code := run([]string{"--config", cfgPath, "--lock", lockPath, "fetch"}); code != 0 {
			t.Fatalf("run(fetch) = %d, want 0", code)
		}
		if code := run([]string{"--config", cfgPath, "--lock", lockPath, "audit"}); code != 0 {
			t.Errorf("run(audit) = %d, want 0", code)
		}
	})

	t.Run("audit with missing config exits 2", func(t *testing.T) {
		tmpDir := t.TempDir()
		code := run([]string{
			"--config", filepath.Join(tmpDir, "missing.yaml"),
			"--lock", filepath.Join(tmpDir, "lock.yaml"),
			"audit",
		})
		if code != 2 {
			t.Errorf("run(audit, missing config) = %d, want 2", code)
		}
	})
}

func TestUsage(t *testing.T) {
	// Just make sure it doesn't panic; output content isn't load-bearing.
	usage()
}

func TestRun_Types(t *testing.T) {
	t.Run("lists source types available in this build", func(t *testing.T) {
		out, code := captureRun(t, []string{"types"})
		if code != 0 {
			t.Fatalf("run(types) = %d, want 0", code)
		}
		for _, name := range []string{"command", "file", "http"} {
			if !strings.Contains(out, name) {
				t.Errorf("types output does not contain %q: %s", name, out)
			}
		}
		if strings.Contains(out, "git-enabled builds") {
			t.Errorf("types output includes optional git handler in a non-git build: %s", out)
		}
	})

	t.Run("prints schema-derived details as JSON", func(t *testing.T) {
		defer func() { core.JSONOutput = false }()
		out, code := captureRun(t, []string{"--json", "types", "http"})
		if code != 0 {
			t.Fatalf("run(--json types http) = %d, want 0", code)
		}
		var report struct {
			Types []struct {
				Type   string `json:"type"`
				Fields []struct {
					Name     string `json:"name"`
					Required bool   `json:"required"`
				} `json:"fields"`
			} `json:"types"`
		}
		if err := json.Unmarshal([]byte(out), &report); err != nil {
			t.Fatalf("invalid JSON output: %v\n%s", err, out)
		}
		if len(report.Types) != 1 || report.Types[0].Type != "http" {
			t.Fatalf("types = %+v, want only http", report.Types)
		}
		if len(report.Types[0].Fields) != 4 || report.Types[0].Fields[1].Name != "url" || !report.Types[0].Fields[1].Required {
			t.Errorf("http fields = %+v, want required type and url from schema", report.Types[0].Fields)
		}
	})

	t.Run("unknown source type exits 2", func(t *testing.T) {
		out, code := captureRun(t, []string{"types", "bogus"})
		if code != 2 {
			t.Fatalf("run(types bogus) = %d, want 2", code)
		}
		if !strings.Contains(out, `unknown dataset source type "bogus"`) {
			t.Errorf("unexpected error output: %s", out)
		}
	})
}

func TestRun_Schema(t *testing.T) {
	out, code := captureRun(t, []string{"schema"})
	if code != 0 {
		t.Fatalf("run(schema) = %d, want 0", code)
	}
	if out != string(datumlib.ConfigSchema()) {
		t.Fatal("datum schema did not print the exact schema shipped with the binary")
	}
	if !json.Valid([]byte(out)) {
		t.Fatal("datum schema output is not valid JSON")
	}
}

func captureRun(t *testing.T, args []string) (string, int) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	original := os.Stdout
	os.Stdout = w
	type readResult struct {
		data []byte
		err  error
	}
	readDone := make(chan readResult, 1)
	go func() {
		data, readErr := io.ReadAll(r)
		readDone <- readResult{data: data, err: readErr}
	}()
	code := run(args)
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	os.Stdout = original
	result := <-readDone
	if err := r.Close(); err != nil {
		t.Fatal(err)
	}
	if result.err != nil {
		t.Fatal(result.err)
	}
	return string(result.data), code
}
