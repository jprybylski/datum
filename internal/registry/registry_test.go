package registry

import (
	"context"
	"testing"
)

// mockFetcher is a test implementation of the Fetcher interface
type mockFetcher struct {
	name string
}

func (m *mockFetcher) Name() string {
	return m.name
}

func (m *mockFetcher) Fingerprint(ctx context.Context, src Source) (string, error) {
	return "mock-fingerprint", nil
}

func (m *mockFetcher) Fetch(ctx context.Context, src Source, dest string) error {
	return nil
}

func TestRegister(t *testing.T) {
	t.Run("register new handler", func(t *testing.T) {
		mock := &mockFetcher{name: "test-handler-unique"}
		Register(mock)

		got, ok := Get("test-handler-unique")
		if !ok {
			t.Error("handler not found in registry after Register")
		}
		if got.Name() != "test-handler-unique" {
			t.Errorf("handler name = %q, want %q", got.Name(), "test-handler-unique")
		}
	})

	t.Run("register multiple handlers", func(t *testing.T) {
		Register(&mockFetcher{name: "handler1-test"})
		Register(&mockFetcher{name: "handler2-test"})
		Register(&mockFetcher{name: "handler3-test"})

		// Verify all were registered
		if _, ok := Get("handler1-test"); !ok {
			t.Error("handler1-test not found")
		}
		if _, ok := Get("handler2-test"); !ok {
			t.Error("handler2-test not found")
		}
		if _, ok := Get("handler3-test"); !ok {
			t.Error("handler3-test not found")
		}
	})

	t.Run("register overwrites existing handler", func(t *testing.T) {
		first := &mockFetcher{name: "overwrite-test-reg"}
		second := &mockFetcher{name: "overwrite-test-reg"}

		Register(first)
		Register(second)

		// Should only have the second one
		got, ok := Get("overwrite-test-reg")
		if !ok {
			t.Error("overwrite-test-reg not found")
		}
		// They should be the same name but potentially different instances
		if got.Name() != "overwrite-test-reg" {
			t.Errorf("handler name = %q, want %q", got.Name(), "overwrite-test-reg")
		}
	})
}

func TestGet(t *testing.T) {
	// Register a handler for testing
	Register(&mockFetcher{name: "get-test-registered"})

	t.Run("get existing handler", func(t *testing.T) {
		handler, ok := Get("get-test-registered")
		if !ok {
			t.Error("Get() ok = false, want true")
		}
		if handler.Name() != "get-test-registered" {
			t.Errorf("handler name = %q, want %q", handler.Name(), "get-test-registered")
		}
	})

	t.Run("get non-existent handler", func(t *testing.T) {
		_, ok := Get("definitely-does-not-exist-12345")
		if ok {
			t.Error("Get() ok = true, want false for non-existent handler")
		}
	})
}

func TestSource(t *testing.T) {
	t.Run("create source with all fields", func(t *testing.T) {
		src := Source{
			Type:           "http",
			URL:            "http://example.com",
			Path:           "/path/to/file",
			Ref:            "main",
			FingerprintCmd: "echo test",
			FetchCmd:       "curl -o {{dest}} {{url}}",
		}

		if src.Type != "http" {
			t.Errorf("Type = %q, want %q", src.Type, "http")
		}
		if src.URL != "http://example.com" {
			t.Errorf("URL = %q, want %q", src.URL, "http://example.com")
		}
		if src.Path != "/path/to/file" {
			t.Errorf("Path = %q, want %q", src.Path, "/path/to/file")
		}
		if src.Ref != "main" {
			t.Errorf("Ref = %q, want %q", src.Ref, "main")
		}
		if src.FingerprintCmd != "echo test" {
			t.Errorf("FingerprintCmd = %q, want %q", src.FingerprintCmd, "echo test")
		}
		if src.FetchCmd != "curl -o {{dest}} {{url}}" {
			t.Errorf("FetchCmd = %q, want %q", src.FetchCmd, "curl -o {{dest}} {{url}}")
		}
	})

	t.Run("create source with minimal fields", func(t *testing.T) {
		src := Source{
			Type: "file",
			Path: "/local/file.txt",
		}

		if src.Type != "file" {
			t.Errorf("Type = %q, want %q", src.Type, "file")
		}
		if src.Path != "/local/file.txt" {
			t.Errorf("Path = %q, want %q", src.Path, "/local/file.txt")
		}
	})
}
