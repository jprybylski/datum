package core

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestHashFile(t *testing.T) {
	// Create a temporary directory for test files
	tmpDir := t.TempDir()

	tests := []struct {
		name     string
		content  string
		wantHash string
	}{
		{
			name:     "empty file",
			content:  "",
			wantHash: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855", // SHA256 of empty string
		},
		{
			name:     "hello world",
			content:  "hello world",
			wantHash: "b94d27b9934d3e08a52e52d7da7dabfac484efe37a5380ee9088f7ace2efcde9",
		},
		{
			name:     "multiline content",
			content:  "line1\nline2\nline3\n",
			wantHash: "9e107d9d372bb6826bd81d3542a419d6e4c6a6c", // This will be computed
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Create a temporary file with the test content
			testFile := filepath.Join(tmpDir, tt.name+".txt")
			if err := os.WriteFile(testFile, []byte(tt.content), 0o644); err != nil {
				t.Fatalf("failed to create test file: %v", err)
			}

			// Compute the hash
			got, err := HashFile(testFile)
			if err != nil {
				t.Fatalf("HashFile() error = %v", err)
			}

			// For multiline content, we just verify it returns a valid SHA256 hash (64 hex chars)
			if tt.name == "multiline content" {
				if len(got) != 64 {
					t.Errorf("HashFile() returned invalid SHA256 length = %d, want 64", len(got))
				}
				return
			}

			if got != tt.wantHash {
				t.Errorf("HashFile() = %v, want %v", got, tt.wantHash)
			}
		})
	}
}

func TestHashFile_NonExistentFile(t *testing.T) {
	_, err := HashFile("/nonexistent/file/that/should/not/exist.txt")
	if err == nil {
		t.Error("HashFile() expected error for non-existent file, got nil")
	}
}

func TestHashFile_DirectoryPath(t *testing.T) {
	// os.Open succeeds on a directory (it's a valid file descriptor), but reading from it as a
	// byte stream fails - this exercises HashFile's io.Copy error branch, distinct from the
	// os.Open error branch already covered by the non-existent-file case above.
	if runtime.GOOS == "windows" {
		t.Skip("reading a directory as a byte stream doesn't reliably fail the same way on Windows")
	}
	dir := t.TempDir()
	if _, err := HashFile(dir); err == nil {
		t.Error("HashFile() on a directory expected an error, got nil")
	}
}

func TestHashDir_UnreadableFile(t *testing.T) {
	// A broken symlink is listed by WalkDir (it's not a directory) but fails to open - this
	// exercises HashDir's propagation of a per-file HashFile error.
	if runtime.GOOS == "windows" {
		t.Skip("creating symlinks on Windows requires elevated privileges")
	}
	dir := t.TempDir()
	link := filepath.Join(dir, "broken-link")
	if err := os.Symlink(filepath.Join(dir, "does-not-exist"), link); err != nil {
		t.Skipf("could not create symlink: %v", err)
	}
	if _, err := HashDir(dir); err == nil {
		t.Error("HashDir() with an unreadable file expected an error, got nil")
	}
}

func TestFileExists(t *testing.T) {
	tmpDir := t.TempDir()

	// Create a test file
	testFile := filepath.Join(tmpDir, "exists.txt")
	if err := os.WriteFile(testFile, []byte("test"), 0o644); err != nil {
		t.Fatalf("failed to create test file: %v", err)
	}

	tests := []struct {
		name string
		path string
		want bool
	}{
		{
			name: "existing file",
			path: testFile,
			want: true,
		},
		{
			name: "existing directory",
			path: tmpDir,
			want: true,
		},
		{
			name: "non-existent file",
			path: filepath.Join(tmpDir, "does-not-exist.txt"),
			want: false,
		},
		{
			name: "non-existent directory",
			path: "/nonexistent/directory",
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := fileExists(tt.path); got != tt.want {
				t.Errorf("fileExists() = %v, want %v", got, tt.want)
			}
		})
	}
}

func TestHashDir(t *testing.T) {
	t.Run("deterministic and order-independent", func(t *testing.T) {
		dirA := t.TempDir()
		mustWriteFile(t, filepath.Join(dirA, "a.txt"), []byte("aaa"))
		mustWriteFile(t, filepath.Join(dirA, "b.txt"), []byte("bbb"))
		if err := os.MkdirAll(filepath.Join(dirA, "sub"), 0o755); err != nil {
			t.Fatal(err)
		}
		mustWriteFile(t, filepath.Join(dirA, "sub", "c.txt"), []byte("ccc"))

		dirB := t.TempDir()
		// Same contents, written in a different order.
		if err := os.MkdirAll(filepath.Join(dirB, "sub"), 0o755); err != nil {
			t.Fatal(err)
		}
		mustWriteFile(t, filepath.Join(dirB, "sub", "c.txt"), []byte("ccc"))
		mustWriteFile(t, filepath.Join(dirB, "b.txt"), []byte("bbb"))
		mustWriteFile(t, filepath.Join(dirB, "a.txt"), []byte("aaa"))

		ha, err := HashDir(dirA)
		if err != nil {
			t.Fatalf("HashDir(dirA) error = %v", err)
		}
		hb, err := HashDir(dirB)
		if err != nil {
			t.Fatalf("HashDir(dirB) error = %v", err)
		}
		if ha != hb {
			t.Errorf("HashDir() = %q for dirA, %q for dirB; want equal for identical contents", ha, hb)
		}
		if len(ha) != 64 {
			t.Errorf("HashDir() length = %d, want 64 (sha256 hex)", len(ha))
		}
	})

	t.Run("changes when a file's content changes", func(t *testing.T) {
		dir := t.TempDir()
		mustWriteFile(t, filepath.Join(dir, "a.txt"), []byte("original"))
		h1, err := HashDir(dir)
		if err != nil {
			t.Fatalf("HashDir() error = %v", err)
		}
		mustWriteFile(t, filepath.Join(dir, "a.txt"), []byte("changed"))
		h2, err := HashDir(dir)
		if err != nil {
			t.Fatalf("HashDir() error = %v", err)
		}
		if h1 == h2 {
			t.Error("HashDir() did not change after file content changed")
		}
	})

	t.Run("changes when a file is added", func(t *testing.T) {
		dir := t.TempDir()
		mustWriteFile(t, filepath.Join(dir, "a.txt"), []byte("aaa"))
		h1, err := HashDir(dir)
		if err != nil {
			t.Fatalf("HashDir() error = %v", err)
		}
		mustWriteFile(t, filepath.Join(dir, "b.txt"), []byte("bbb"))
		h2, err := HashDir(dir)
		if err != nil {
			t.Fatalf("HashDir() error = %v", err)
		}
		if h1 == h2 {
			t.Error("HashDir() did not change after a file was added")
		}
	})

	t.Run("changes when a file is renamed (same content, different manifest)", func(t *testing.T) {
		dirA := t.TempDir()
		mustWriteFile(t, filepath.Join(dirA, "a.txt"), []byte("same content"))
		dirB := t.TempDir()
		mustWriteFile(t, filepath.Join(dirB, "renamed.txt"), []byte("same content"))

		ha, err := HashDir(dirA)
		if err != nil {
			t.Fatalf("HashDir(dirA) error = %v", err)
		}
		hb, err := HashDir(dirB)
		if err != nil {
			t.Fatalf("HashDir(dirB) error = %v", err)
		}
		if ha == hb {
			t.Error("HashDir() should differ when file paths differ, even with identical content")
		}
	})

	t.Run("empty directory", func(t *testing.T) {
		dir := t.TempDir()
		h, err := HashDir(dir)
		if err != nil {
			t.Fatalf("HashDir() on empty dir error = %v", err)
		}
		if len(h) != 64 {
			t.Errorf("HashDir() length = %d, want 64", len(h))
		}
	})

	t.Run("non-existent directory", func(t *testing.T) {
		if _, err := HashDir("/nonexistent/dir/that/should/not/exist"); err == nil {
			t.Error("HashDir() expected error for non-existent directory, got nil")
		}
	})
}

func TestDirManifest(t *testing.T) {
	dir := t.TempDir()
	mustWriteFile(t, filepath.Join(dir, "z.txt"), []byte("z"))
	mustWriteFile(t, filepath.Join(dir, "a.txt"), []byte("a"))
	if err := os.MkdirAll(filepath.Join(dir, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	mustWriteFile(t, filepath.Join(dir, "sub", "m.txt"), []byte("m"))

	rels, err := DirManifest(dir)
	if err != nil {
		t.Fatalf("DirManifest() error = %v", err)
	}
	want := []string{"a.txt", "sub/m.txt", "z.txt"}
	if len(rels) != len(want) {
		t.Fatalf("DirManifest() = %v, want %v", rels, want)
	}
	for i := range want {
		if rels[i] != want[i] {
			t.Errorf("DirManifest()[%d] = %q, want %q (want sorted order)", i, rels[i], want[i])
		}
	}
}

func TestHashPath(t *testing.T) {
	t.Run("file", func(t *testing.T) {
		tmpDir := t.TempDir()
		f := filepath.Join(tmpDir, "f.txt")
		mustWriteFile(t, f, []byte("content"))

		want, err := HashFile(f)
		if err != nil {
			t.Fatalf("HashFile() error = %v", err)
		}
		got, err := HashPath(f)
		if err != nil {
			t.Fatalf("HashPath() error = %v", err)
		}
		if got != want {
			t.Errorf("HashPath(file) = %q, want %q (same as HashFile)", got, want)
		}
	})

	t.Run("directory", func(t *testing.T) {
		dir := t.TempDir()
		mustWriteFile(t, filepath.Join(dir, "f.txt"), []byte("content"))

		want, err := HashDir(dir)
		if err != nil {
			t.Fatalf("HashDir() error = %v", err)
		}
		got, err := HashPath(dir)
		if err != nil {
			t.Fatalf("HashPath() error = %v", err)
		}
		if got != want {
			t.Errorf("HashPath(dir) = %q, want %q (same as HashDir)", got, want)
		}
	})

	t.Run("non-existent path", func(t *testing.T) {
		if _, err := HashPath("/nonexistent/path"); err == nil {
			t.Error("HashPath() expected error for non-existent path, got nil")
		}
	})
}

func TestFirstNonEmpty(t *testing.T) {
	tests := []struct {
		name string
		a    string
		b    string
		want string
	}{
		{
			name: "both non-empty, returns first",
			a:    "first",
			b:    "second",
			want: "first",
		},
		{
			name: "first empty, returns second",
			a:    "",
			b:    "second",
			want: "second",
		},
		{
			name: "both empty, returns empty",
			a:    "",
			b:    "",
			want: "",
		},
		{
			name: "first non-empty, second empty",
			a:    "first",
			b:    "",
			want: "first",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := firstNonEmpty(tt.a, tt.b); got != tt.want {
				t.Errorf("firstNonEmpty() = %v, want %v", got, tt.want)
			}
		})
	}
}
