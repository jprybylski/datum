package command

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"example.com/datum/internal/registry"
)

func TestHandler_Name(t *testing.T) {
	h := New()
	if got := h.Name(); got != "command" {
		t.Errorf("Name() = %v, want command", got)
	}
}

func TestHandler_Fingerprint(t *testing.T) {
	ctx := context.Background()
	h := New()

	t.Run("successful fingerprint", func(t *testing.T) {
		src := registry.Source{
			FingerprintCmd: "echo test-fingerprint",
		}

		fp, err := h.Fingerprint(ctx, src)
		if err != nil {
			t.Fatalf("Fingerprint() error = %v", err)
		}
		if fp != "test-fingerprint" {
			t.Errorf("Fingerprint() = %q, want %q", fp, "test-fingerprint")
		}
	})

	t.Run("fingerprint with command", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("skipping Unix-specific test on Windows")
		}
		src := registry.Source{
			FingerprintCmd: "date +%Y-%m-%d",
		}

		fp, err := h.Fingerprint(ctx, src)
		if err != nil {
			t.Fatalf("Fingerprint() error = %v", err)
		}
		// Just verify it returns something (date format validation would be fragile)
		if len(fp) == 0 {
			t.Error("Fingerprint() returned empty string")
		}
	})

	t.Run("missing fingerprint_cmd", func(t *testing.T) {
		src := registry.Source{}

		_, err := h.Fingerprint(ctx, src)
		if err == nil {
			t.Error("Fingerprint() expected error for missing fingerprint_cmd, got nil")
		}
	})

	t.Run("empty fingerprint_cmd", func(t *testing.T) {
		src := registry.Source{
			FingerprintCmd: "   ",
		}

		_, err := h.Fingerprint(ctx, src)
		if err == nil {
			t.Error("Fingerprint() expected error for empty fingerprint_cmd, got nil")
		}
	})

	t.Run("failed command", func(t *testing.T) {
		src := registry.Source{
			FingerprintCmd: "exit 1",
		}

		_, err := h.Fingerprint(ctx, src)
		if err == nil {
			t.Error("Fingerprint() expected error for failed command, got nil")
		}
	})
}

func TestHandler_Fetch(t *testing.T) {
	tmpDir := t.TempDir()
	ctx := context.Background()
	h := New()

	t.Run("successful fetch", func(t *testing.T) {
		destFile := filepath.Join(tmpDir, "output1.txt")
		src := registry.Source{
			FetchCmd: "echo fetched content > {{dest}}",
		}

		err := h.Fetch(ctx, src, destFile)
		if err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}

		// Verify file was created
		content, err := os.ReadFile(destFile)
		if err != nil {
			t.Fatalf("failed to read output file: %v", err)
		}
		// Check for "fetched content" with flexible line ending
		contentStr := string(content)
		if contentStr != "fetched content\n" && contentStr != "fetched content\r\n" {
			t.Errorf("Fetch() content = %q, want %q or %q", contentStr, "fetched content\n", "fetched content\r\n")
		}
	})

	t.Run("fetch with DEST env variable", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("skipping Unix-specific test on Windows")
		}
		destFile := filepath.Join(tmpDir, "subdir", "output2.txt")
		src := registry.Source{
			FetchCmd: "mkdir -p $(dirname $DEST) && echo 'env test' > $DEST",
		}

		err := h.Fetch(ctx, src, destFile)
		if err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}

		// Verify file was created
		if _, err := os.Stat(destFile); err != nil {
			t.Errorf("Fetch() did not create file: %v", err)
		}
	})

	t.Run("fetch with template substitution", func(t *testing.T) {
		destFile := filepath.Join(tmpDir, "output3.txt")
		src := registry.Source{
			URL:      "http://example.com",
			Path:     "/some/path",
			Ref:      "v1.0.0",
			FetchCmd: "echo url={{url}} path={{path}} ref={{ref}} > {{dest}}",
		}

		err := h.Fetch(ctx, src, destFile)
		if err != nil {
			t.Fatalf("Fetch() error = %v", err)
		}

		// Verify substitutions worked
		content, err := os.ReadFile(destFile)
		if err != nil {
			t.Fatalf("failed to read output file: %v", err)
		}
		contentStr := string(content)
		expected1 := "url=http://example.com path=/some/path ref=v1.0.0\n"
		expected2 := "url=http://example.com path=/some/path ref=v1.0.0\r\n"
		if contentStr != expected1 && contentStr != expected2 {
			t.Errorf("Fetch() content = %q, want %q or %q", contentStr, expected1, expected2)
		}
	})

	t.Run("missing fetch_cmd", func(t *testing.T) {
		src := registry.Source{}

		err := h.Fetch(ctx, src, filepath.Join(tmpDir, "output.txt"))
		if err == nil {
			t.Error("Fetch() expected error for missing fetch_cmd, got nil")
		}
	})

	t.Run("empty fetch_cmd", func(t *testing.T) {
		src := registry.Source{
			FetchCmd: "   ",
		}

		err := h.Fetch(ctx, src, filepath.Join(tmpDir, "output.txt"))
		if err == nil {
			t.Error("Fetch() expected error for empty fetch_cmd, got nil")
		}
	})

	t.Run("failed command", func(t *testing.T) {
		src := registry.Source{
			FetchCmd: "exit 1",
		}

		err := h.Fetch(ctx, src, filepath.Join(tmpDir, "output.txt"))
		if err == nil {
			t.Error("Fetch() expected error for failed command, got nil")
		}
	})
}

func TestSubstitute(t *testing.T) {
	tests := []struct {
		name string
		tmpl string
		src  registry.Source
		dest string
		want string
	}{
		{
			name: "substitute url",
			tmpl: "curl {{url}}",
			src:  registry.Source{URL: "http://example.com"},
			dest: "/tmp/file",
			want: "curl http://example.com",
		},
		{
			name: "substitute path",
			tmpl: "cp {{path}} {{dest}}",
			src:  registry.Source{Path: "/src/file.txt"},
			dest: "/dst/file.txt",
			want: "cp /src/file.txt /dst/file.txt",
		},
		{
			name: "substitute ref",
			tmpl: "git checkout {{ref}}",
			src:  registry.Source{Ref: "main"},
			dest: "",
			want: "git checkout main",
		},
		{
			name: "substitute all",
			tmpl: "{{url}} {{path}} {{ref}} {{dest}}",
			src:  registry.Source{URL: "u", Path: "p", Ref: "r"},
			dest: "d",
			want: "u p r d",
		},
		{
			name: "no substitution",
			tmpl: "echo hello",
			src:  registry.Source{},
			dest: "",
			want: "echo hello",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := substitute(tt.tmpl, tt.src, tt.dest)
			if got != tt.want {
				t.Errorf("substitute() = %q, want %q", got, tt.want)
			}
		})
	}
}
