package core

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/jprybylski/datum/internal/registry"
)

func TestReplaceGitIgnoreBlock(t *testing.T) {
	existing := []byte("vendor/\n\n" + gitIgnoreBegin + "\n/old.csv\n" + gitIgnoreEnd + "\n")
	got, err := replaceGitIgnoreBlock(existing, []string{"/data/b.csv", "/data/a.csv"})
	if err != nil {
		t.Fatal(err)
	}
	want := "vendor/\n\n" + gitIgnoreBegin + "\n/data/a.csv\n/data/b.csv\n" + gitIgnoreEnd + "\n"
	if string(got) != want {
		t.Fatalf("ignore content:\n%s\nwant:\n%s", got, want)
	}
	got, err = replaceGitIgnoreBlock(got, nil)
	if err != nil || string(got) != "vendor/\n" {
		t.Fatalf("removed block = %q, %v", got, err)
	}
}

func TestGitIgnorePatternEscapesMetacharacters(t *testing.T) {
	if got, want := gitIgnorePattern("data/a [final]*.csv"), `/data/a\ \[final\]\*.csv`; got != want {
		t.Fatalf("gitIgnorePattern() = %q, want %q", got, want)
	}
}

func TestReconcileIgnoresOutsideWorkingCopyIsNoop(t *testing.T) {
	enterTempDir(t)
	original := runVCSCommand
	t.Cleanup(func() { runVCSCommand = original })
	runVCSCommand = func(string, ...string) ([]byte, error) {
		t.Fatal("VCS command called outside a working copy")
		return nil, nil
	}
	if err := reconcileIgnores(ignoreTestConfig(true)); err != nil {
		t.Fatal(err)
	}
}

func TestReconcileGitIgnore(t *testing.T) {
	dir := enterTempDir(t)
	runCommand(t, "git", "init", "--quiet", dir)
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("vendor/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := ignoreTestConfig(true)
	if err := reconcileIgnores(cfg); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "vendor/\n") || !strings.Contains(string(content), "/data/example.csv") {
		t.Fatalf(".gitignore = %q", content)
	}
	if err := reconcileIgnores(cfg); err != nil {
		t.Fatal(err)
	}
	cfg.Defaults.Ignore = false
	if err := reconcileIgnores(cfg); err != nil {
		t.Fatal(err)
	}
	content, err = os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil || string(content) != "vendor/\n" {
		t.Fatalf("cleaned .gitignore = %q, %v", content, err)
	}
}

func TestReconcileGitIgnoreRejectsTrackedTarget(t *testing.T) {
	dir := enterTempDir(t)
	runCommand(t, "git", "init", "--quiet", dir)
	if err := os.MkdirAll(filepath.Join(dir, "data"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "data", "example.csv"), []byte("tracked"), 0o644); err != nil {
		t.Fatal(err)
	}
	runCommand(t, "git", "-C", dir, "add", "data/example.csv")
	if err := reconcileIgnores(ignoreTestConfig(true)); err == nil || !strings.Contains(err.Error(), "already tracked by Git") {
		t.Fatalf("reconcile error = %v", err)
	}
}

func TestPrepareAndApplySVNIgnore(t *testing.T) {
	dir := enterTempDir(t)
	dataDir := filepath.Join(dir, "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	original := runVCSCommand
	t.Cleanup(func() { runVCSCommand = original })
	written := map[string]string{}
	runVCSCommand = func(name string, args ...string) ([]byte, error) {
		if name != "svn" {
			t.Fatalf("command = %s, want svn", name)
		}
		switch args[0] {
		case "propget":
			return []byte("<?xml version=\"1.0\"?><properties></properties>"), nil
		case "info":
			if filepath.Clean(args[1]) == dataDir {
				return []byte("versioned parent"), nil
			}
			return nil, errors.New("unversioned target")
		case "propset":
			content, err := os.ReadFile(args[3])
			if err != nil {
				t.Fatal(err)
			}
			written[args[1]] = string(content)
			return nil, nil
		default:
			t.Fatalf("unexpected svn args: %v", args)
			return nil, nil
		}
	}
	plan, err := prepareSVNIgnore(dir, dir, ignoreTestConfig(true))
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.apply(); err != nil {
		t.Fatal(err)
	}
	if written["svn:ignore"] != "example.csv\n" || written[svnOwnedProp] != "example.csv\n" {
		t.Fatalf("written SVN properties = %#v", written)
	}
}

func ignoreTestConfig(ignore bool) *Config {
	return &Config{
		Version: 1, Defaults: Defaults{Policy: "fail", Algo: "sha256", Ignore: ignore},
		Datasets: []Dataset{{
			ID: "example", Target: "data/example.csv",
			Source: registry.Source{Type: "http", URL: "https://example.com/data.csv"},
		}},
	}
}

func runCommand(t *testing.T, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v: %v: %s", name, args, err, output)
	}
}
