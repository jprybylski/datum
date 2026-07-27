package main

import (
	"os"
	"path/filepath"
	"testing"
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
}

func TestUsage(t *testing.T) {
	// Just make sure it doesn't panic; output content isn't load-bearing.
	usage()
}
