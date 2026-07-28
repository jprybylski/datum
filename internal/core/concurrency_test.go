package core

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jprybylski/datum/internal/registry"
)

// mockSlowHandler simulates a handler whose fingerprint/fetch takes `delay`, and respects ctx
// cancellation - used to test that concurrency actually overlaps work and that --timeout's
// context deadline actually cuts work short instead of just being accepted and ignored.
type mockSlowHandler struct {
	name  string
	delay time.Duration
}

func (m *mockSlowHandler) Name() string { return m.name }

func (m *mockSlowHandler) Fingerprint(ctx context.Context, src registry.Source) (string, error) {
	select {
	case <-time.After(m.delay):
		return m.name + "-fp", nil
	case <-ctx.Done():
		return "", ctx.Err()
	}
}

func (m *mockSlowHandler) Fetch(ctx context.Context, src registry.Source, dest string) error {
	select {
	case <-time.After(m.delay):
		return os.WriteFile(dest, []byte(m.name), 0o644)
	case <-ctx.Done():
		return ctx.Err()
	}
}

func init() {
	registry.Register(&mockSlowHandler{name: "slow100ms", delay: 100 * time.Millisecond})
	registry.Register(&mockSlowHandler{name: "slowA30ms", delay: 30 * time.Millisecond})
	registry.Register(&mockSlowHandler{name: "slowB10ms", delay: 10 * time.Millisecond})
	registry.Register(&mockSlowHandler{name: "slowC20ms", delay: 20 * time.Millisecond})
}

func writeConfigWithSlowDatasets(t *testing.T, path string, n int, sourceType string, targetDir string) {
	t.Helper()
	cfg := "version: 1\ndatasets:\n"
	for i := 0; i < n; i++ {
		id := string(rune('a' + i))
		cfg += "  - id: ds" + id + "\n    source:\n      type: " + sourceType + "\n    target: " + filepath.Join(targetDir, "ds"+id+".txt") + "\n"
	}
	mustWriteFile(t, path, []byte(cfg))
}

func TestFetch_ConcurrencySpeedsUpProcessing(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	writeConfigWithSlowDatasets(t, configPath, 4, "slow100ms", tmpDir)

	t.Run("sequential", func(t *testing.T) {
		lockPath := filepath.Join(tmpDir, "seq.lock.yaml")
		start := time.Now()
		if code := Fetch(context.Background(), configPath, lockPath, nil, 1); code != 0 {
			t.Fatalf("Fetch() = %d, want 0", code)
		}
		elapsedSequential := time.Since(start)

		lockPath2 := filepath.Join(tmpDir, "par.lock.yaml")
		start = time.Now()
		if code := Fetch(context.Background(), configPath, lockPath2, nil, 4); code != 0 {
			t.Fatalf("Fetch() = %d, want 0", code)
		}
		elapsedParallel := time.Since(start)

		// 4 datasets at 100ms each: sequential should take ~400ms, concurrency=4 should take
		// ~100ms. Assert a generous margin (parallel takes less than 70% of sequential) to
		// avoid flaking under CI scheduling jitter while still catching "concurrency did nothing".
		if elapsedParallel >= elapsedSequential*7/10 {
			t.Errorf("concurrency=4 (%v) was not meaningfully faster than concurrency=1 (%v)", elapsedParallel, elapsedSequential)
		}
	})
}

func TestFetch_OutputOrderMatchesConfigOrder(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	// Deliberately ordered so the fastest (slowB, 10ms) finishes first and the slowest
	// (slowA, 30ms) finishes last when run concurrently - but config order is A, B, C.
	cfg := `version: 1
datasets:
  - id: dsA
    source:
      type: slowA30ms
    target: ` + filepath.Join(tmpDir, "dsA.txt") + `
  - id: dsB
    source:
      type: slowB10ms
    target: ` + filepath.Join(tmpDir, "dsB.txt") + `
  - id: dsC
    source:
      type: slowC20ms
    target: ` + filepath.Join(tmpDir, "dsC.txt") + `
`
	mustWriteFile(t, configPath, []byte(cfg))
	lockPath := filepath.Join(tmpDir, "lock.yaml")

	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	origStdout := os.Stdout
	os.Stdout = w
	code := Fetch(context.Background(), configPath, lockPath, nil, 3)
	os.Stdout = origStdout
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if code != 0 {
		t.Fatalf("Fetch() = %d, want 0", code)
	}

	buf := make([]byte, 4096)
	n, _ := r.Read(buf)
	output := string(buf[:n])

	idxA := indexOf(output, "[FETCH] dsA")
	idxB := indexOf(output, "[FETCH] dsB")
	idxC := indexOf(output, "[FETCH] dsC")
	if idxA < 0 || idxB < 0 || idxC < 0 {
		t.Fatalf("expected [FETCH] lines for all three datasets in output:\n%s", output)
	}
	if !(idxA < idxB && idxB < idxC) {
		t.Errorf("output not in config order (want A < B < C): A=%d B=%d C=%d\noutput:\n%s", idxA, idxB, idxC, output)
	}
}

func indexOf(s, substr string) int {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return i
		}
	}
	return -1
}

func TestCheck_ContextTimeoutCutsWorkShort(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, "config.yaml")
	configContent := `version: 1
datasets:
  - id: slow_dataset
    source:
      type: slow100ms
    target: ` + filepath.Join(tmpDir, "target.txt") + `
    policy: fail
`
	mustWriteFile(t, configPath, []byte(configContent))
	lockPath := filepath.Join(tmpDir, "lock.yaml")

	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Millisecond)
	defer cancel()

	start := time.Now()
	code := Check(ctx, configPath, lockPath, 1)
	elapsed := time.Since(start)

	if code != 1 {
		t.Errorf("Check() with expired context = %d, want 1 (fingerprint should fail via ctx.Done())", code)
	}
	// The handler's own delay is 100ms; if the context deadline wasn't actually propagated and
	// respected, this would take ~100ms instead of stopping at ~20ms.
	if elapsed > 80*time.Millisecond {
		t.Errorf("Check() took %v, expected it to stop well before the handler's 100ms delay once the context timeout (20ms) expired", elapsed)
	}
}
