package core

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// captureStdout runs fn with os.Stdout redirected to a pipe and returns everything written to it.
// Check/Fetch print directly to os.Stdout (not an injectable io.Writer), so this is the only way
// to observe --json's final printed document.
func captureStdout(t *testing.T, fn func()) string {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stdout
	os.Stdout = w
	fn()
	os.Stdout = orig
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	buf := make([]byte, 64*1024)
	n, _ := r.Read(buf)
	return string(buf[:n])
}

// withJSONOutput sets JSONOutput for the duration of fn and restores it afterward, since it's
// package-level state shared with every other test in this package.
func withJSONOutput(t *testing.T, fn func()) {
	t.Helper()
	JSONOutput = true
	t.Cleanup(func() { JSONOutput = false })
	fn()
}

func TestCheck_JSONOutput_OK(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	targetFile := filepath.Join(tmpDir, "target.txt")
	lockPath := filepath.Join(tmpDir, "lock.yaml")

	mustWriteFile(t, configPath, []byte(`version: 1
datasets:
  - id: demo
    source:
      type: mock
    target: `+targetFile+`
    policy: fail
`))
	mustWriteFile(t, lockPath, []byte(`version: 1
items:
  demo:
    remote_fingerprint: mock-fp
`))

	var out string
	var code int
	withJSONOutput(t, func() {
		out = captureStdout(t, func() {
			code = Check(context.Background(), configPath, lockPath, 1)
		})
	})
	if code != 0 {
		t.Fatalf("Check() = %d, want 0", code)
	}

	var report Report
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("output isn't valid JSON: %v\noutput: %s", err, out)
	}
	if len(report.Results) != 1 {
		t.Fatalf("len(Results) = %d, want 1", len(report.Results))
	}
	r := report.Results[0]
	if r.ID != "demo" || r.Status != StatusOK {
		t.Errorf("Results[0] = %+v, want ID=demo Status=ok", r)
	}
	if r.LockFingerprint != "mock-fp" || r.RemoteFingerprint != "mock-fp" {
		t.Errorf("Results[0] fingerprints = %+v, want both mock-fp", r)
	}
}

func TestCheck_JSONOutput_Fail(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	targetFile := filepath.Join(tmpDir, "target.txt")
	lockPath := filepath.Join(tmpDir, "lock.yaml")

	mustWriteFile(t, configPath, []byte(`version: 1
datasets:
  - id: demo
    source:
      type: mock
    target: `+targetFile+`
    policy: fail
`))
	mustWriteFile(t, lockPath, []byte(`version: 1
items:
  demo:
    remote_fingerprint: old_fingerprint
`))

	var out string
	var code int
	withJSONOutput(t, func() {
		out = captureStdout(t, func() {
			code = Check(context.Background(), configPath, lockPath, 1)
		})
	})
	if code != 1 {
		t.Fatalf("Check() = %d, want 1", code)
	}

	var report Report
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("output isn't valid JSON: %v\noutput: %s", err, out)
	}
	r := report.Results[0]
	if r.Status != StatusFail {
		t.Errorf("Status = %q, want %q", r.Status, StatusFail)
	}
	if r.LockFingerprint != "old_fingerprint" || r.RemoteFingerprint != "mock-fp" {
		t.Errorf("fingerprints = lock:%q remote:%q, want lock:old_fingerprint remote:mock-fp", r.LockFingerprint, r.RemoteFingerprint)
	}
}

func TestFetch_JSONOutput(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	targetFile := filepath.Join(tmpDir, "target.txt")
	lockPath := filepath.Join(tmpDir, "lock.yaml")

	mustWriteFile(t, configPath, []byte(`version: 1
datasets:
  - id: demo
    source:
      type: mock
    target: `+targetFile+`
`))

	var out string
	var code int
	withJSONOutput(t, func() {
		out = captureStdout(t, func() {
			code = Fetch(context.Background(), configPath, lockPath, nil, 1)
		})
	})
	if code != 0 {
		t.Fatalf("Fetch() = %d, want 0", code)
	}

	var report Report
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("output isn't valid JSON: %v\noutput: %s", err, out)
	}
	r := report.Results[0]
	if r.ID != "demo" || r.Status != StatusFetched || r.RemoteFingerprint != "mock-fp" {
		t.Errorf("Results[0] = %+v, want ID=demo Status=fetched RemoteFingerprint=mock-fp", r)
	}
}

func TestCheck_JSONOutput_ConfigError(t *testing.T) {
	tmpDir := t.TempDir()

	var out string
	var code int
	withJSONOutput(t, func() {
		out = captureStdout(t, func() {
			code = Check(context.Background(), filepath.Join(tmpDir, "missing.yaml"), filepath.Join(tmpDir, "lock.yaml"), 1)
		})
	})
	if code != 2 {
		t.Fatalf("Check() = %d, want 2", code)
	}

	var errResp struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal([]byte(out), &errResp); err != nil {
		t.Fatalf("output isn't valid JSON: %v\noutput: %s", err, out)
	}
	if errResp.Error == "" {
		t.Errorf("expected a non-empty error message, got %q", out)
	}
}
