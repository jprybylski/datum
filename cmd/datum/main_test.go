package main

import (
	"os"
	"path/filepath"
	"testing"

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

func TestUsage(t *testing.T) {
	// Just make sure it doesn't panic; output content isn't load-bearing.
	usage()
}
