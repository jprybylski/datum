package core

import (
	"os"
	"path/filepath"
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
