package core

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func writeTree(t *testing.T, dir string, files map[string]string) {
	t.Helper()
	for rel, content := range files {
		path := filepath.Join(dir, filepath.FromSlash(rel))
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func readTree(t *testing.T, dir string) map[string]string {
	t.Helper()
	rels, err := DirManifest(dir)
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{}
	for _, rel := range rels {
		b, err := os.ReadFile(filepath.Join(dir, filepath.FromSlash(rel)))
		if err != nil {
			t.Fatal(err)
		}
		files[rel] = string(b)
	}
	return files
}

func sameFiles(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

// An interrupted swap, once recovered, leaves the old tree or the new one - never a mix (#38).
func TestReplaceDir_InterruptedAtEveryStepRecoversToOldOrNew(t *testing.T) {
	oldFiles := map[string]string{"a.txt": "old a", "sub/b.txt": "old b"}
	newFiles := map[string]string{"a.txt": "new a", "c.txt": "new c"}
	for _, step := range []string{"moved-old", "moved-new", ""} {
		t.Run("stop after "+step, func(t *testing.T) {
			dest := filepath.Join(t.TempDir(), "target")
			writeTree(t, dest, oldFiles)
			built, err := NewSiblingDir(dest)
			if err != nil {
				t.Fatal(err)
			}
			writeTree(t, built, newFiles)

			stop := errors.New("killed")
			dirSwapHook = func(s string) error {
				if s == step {
					return stop
				}
				return nil
			}
			defer func() { dirSwapHook = func(string) error { return nil } }()
			err = ReplaceDir(built, dest)
			if step != "" && !errors.Is(err, stop) {
				t.Fatalf("ReplaceDir() = %v, want the injected stop", err)
			}

			restored, err := RecoverDirSwap(dest)
			if err != nil {
				t.Fatal(err)
			}
			got := readTree(t, dest)
			switch step {
			case "moved-old":
				if !restored || !sameFiles(got, oldFiles) {
					t.Errorf("restored=%v tree=%v, want the old tree back", restored, got)
				}
			default:
				if restored || !sameFiles(got, newFiles) {
					t.Errorf("restored=%v tree=%v, want the new tree", restored, got)
				}
			}
			entries, _ := os.ReadDir(filepath.Dir(dest))
			if len(entries) != 1 {
				t.Errorf("leftovers next to the target: %v", entries)
			}
		})
	}
}

func TestRecoverDirSwap_NothingToDo(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "target")
	if restored, err := RecoverDirSwap(dest); restored || err != nil {
		t.Errorf("RecoverDirSwap(no folder) = %v, %v", restored, err)
	}
	if restored, err := RecoverDirSwap(filepath.Join(t.TempDir(), "missing-parent", "x")); restored || err != nil {
		t.Errorf("RecoverDirSwap(missing parent) = %v, %v", restored, err)
	}
	if restored, err := RecoverDirSwap(""); restored || err != nil {
		t.Errorf("RecoverDirSwap(\"\") = %v, %v", restored, err)
	}
}

func TestNewSiblingDir_KeepsTheOldFoldersMode(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "target")
	if err := os.Mkdir(dest, 0o750); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dest, 0o750); err != nil {
		t.Fatal(err)
	}
	built, err := NewSiblingDir(dest)
	if err != nil {
		t.Fatal(err)
	}
	a, _ := os.Stat(dest)
	b, _ := os.Stat(built)
	if a.Mode().Perm() != b.Mode().Perm() {
		t.Errorf("mode = %v, want %v", b.Mode().Perm(), a.Mode().Perm())
	}
}
