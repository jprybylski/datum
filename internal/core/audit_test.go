package core

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
)

func TestAudit_ConfigError(t *testing.T) {
	tmpDir := t.TempDir()
	out := captureStdout(t, func() {
		code := Audit(filepath.Join(tmpDir, "missing.yaml"), filepath.Join(tmpDir, ".data.lock.yaml"))
		if code != 2 {
			t.Errorf("Audit() with missing config = %d, want 2", code)
		}
	})
	if !strings.Contains(out, "config error") {
		t.Errorf("output = %q, want it to mention a config error", out)
	}
}

func TestAudit_LockError(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".data.yaml")
	mustWriteFile(t, configPath, []byte("version: 1\ndatasets: []\n"))
	lockPath := filepath.Join(tmpDir, "bad.lock.yaml")
	mustWriteFile(t, lockPath, []byte("bad: yaml: syntax:"))

	out := captureStdout(t, func() {
		code := Audit(configPath, lockPath)
		if code != 2 {
			t.Errorf("Audit() with invalid lock = %d, want 2", code)
		}
	})
	if !strings.Contains(out, "lock error") {
		t.Errorf("output = %q, want it to mention a lock error", out)
	}
}

func TestAudit_NoDatasets(t *testing.T) {
	tmpDir := t.TempDir()
	configPath := filepath.Join(tmpDir, ".data.yaml")
	lockPath := filepath.Join(tmpDir, ".data.lock.yaml")
	mustWriteFile(t, configPath, []byte("version: 1\ndatasets: []\n"))

	out := captureStdout(t, func() {
		if code := Audit(configPath, lockPath); code != 0 {
			t.Errorf("Audit() = %d, want 0", code)
		}
	})
	if !strings.Contains(out, "no datasets tracked") {
		t.Errorf("output = %q, want it to say no datasets tracked", out)
	}
}

// auditFixture builds a config+lockfile combination exercising all four AuditStatus values:
// ds_ok (fetched, tracked), ds_pending (tracked, never fetched), ds_deleted (tracked but
// datum-deleted), and ds_orphan (lockfile-only, no longer in config).
func auditFixture(t *testing.T) (configPath, lockPath string) {
	t.Helper()
	tmpDir := t.TempDir()
	configPath = filepath.Join(tmpDir, ".data.yaml")
	lockPath = filepath.Join(tmpDir, ".data.lock.yaml")

	mustWriteFile(t, configPath, []byte(`version: 1
defaults:
  policy: update
datasets:
  - id: ds_ok
    source:
      type: mock
    target: `+filepath.Join(tmpDir, "ok.txt")+`
  - id: ds_pending
    source:
      type: mock
    target: `+filepath.Join(tmpDir, "pending.txt")+`
  - id: ds_deleted
    source:
      type: mock
    target: `+filepath.Join(tmpDir, "deleted.txt")+`
`))

	if code := Fetch(context.Background(), configPath, lockPath, []string{"ds_ok", "ds_deleted"}, 1); code != 0 {
		t.Fatalf("Fetch() setup = %d, want 0", code)
	}
	var out strings.Builder
	if code := Delete(configPath, lockPath, []string{"ds_deleted"}, true, nil, &out); code != 0 {
		t.Fatalf("Delete() setup = %d, want 0", code)
	}

	lk, err := readLock(lockPath)
	if err != nil {
		t.Fatalf("readLock() error = %v", err)
	}
	lk.Items["ds_orphan"] = &LockItem{RemoteFingerprint: "orphan-fp"}
	if err := writeLock(lockPath, lk); err != nil {
		t.Fatalf("writeLock() error = %v", err)
	}

	return configPath, lockPath
}

func TestAudit_TextOutput(t *testing.T) {
	configPath, lockPath := auditFixture(t)

	out := captureStdout(t, func() {
		if code := Audit(configPath, lockPath); code != 0 {
			t.Errorf("Audit() = %d, want 0", code)
		}
	})

	for _, want := range []string{
		"[OK  ] ds_ok",
		"[PEND] ds_pending",
		"never fetched",
		"[DEL ] ds_deleted",
		"still in .data.yaml; run 'datum undelete ds_deleted'",
		"[ORPH] ds_orphan",
		"not in .data.yaml; run 'datum unlock ds_orphan'",
		"4 dataset(s): 1 ok, 1 pending, 1 deleted, 1 orphaned",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("output missing %q\nfull output:\n%s", want, out)
		}
	}

	// Config-declared order first (ds_ok, ds_pending, ds_deleted), orphaned entries after.
	okIdx := strings.Index(out, "ds_ok")
	pendingIdx := strings.Index(out, "ds_pending")
	deletedIdx := strings.Index(out, "ds_deleted")
	orphanIdx := strings.Index(out, "ds_orphan")
	if !(okIdx < pendingIdx && pendingIdx < deletedIdx && deletedIdx < orphanIdx) {
		t.Errorf("entries out of expected order: ok=%d pending=%d deleted=%d orphan=%d", okIdx, pendingIdx, deletedIdx, orphanIdx)
	}
}

func TestAudit_JSONOutput(t *testing.T) {
	configPath, lockPath := auditFixture(t)

	var out string
	withJSONOutput(t, func() {
		out = captureStdout(t, func() {
			if code := Audit(configPath, lockPath); code != 0 {
				t.Errorf("Audit() = %d, want 0", code)
			}
		})
	})

	var report AuditReport
	if err := json.Unmarshal([]byte(out), &report); err != nil {
		t.Fatalf("output isn't valid JSON: %v\noutput: %s", err, out)
	}
	if len(report.Entries) != 4 {
		t.Fatalf("len(Entries) = %d, want 4", len(report.Entries))
	}

	byID := map[string]AuditEntry{}
	for _, e := range report.Entries {
		byID[e.ID] = e
	}

	if e := byID["ds_ok"]; e.Status != AuditOK || !e.InConfig || e.RemoteFingerprint == "" {
		t.Errorf("ds_ok = %+v, want Status=ok InConfig=true with a fingerprint", e)
	}
	if e := byID["ds_pending"]; e.Status != AuditPending || !e.InConfig {
		t.Errorf("ds_pending = %+v, want Status=pending InConfig=true", e)
	}
	if e := byID["ds_deleted"]; e.Status != AuditDeleted || !e.InConfig || e.DeletedAt == nil {
		t.Errorf("ds_deleted = %+v, want Status=deleted InConfig=true with DeletedAt set", e)
	}
	if e := byID["ds_orphan"]; e.Status != AuditOrphaned || e.InConfig {
		t.Errorf("ds_orphan = %+v, want Status=orphaned InConfig=false", e)
	}
}
