package core

import (
	"encoding/json"
	"fmt"
)

// ReportFormat identifies the shape of every JSON document datum prints (except `datum schema`,
// which is the configuration schema itself). It's bumped only on an incompatible change; new
// optional fields don't change it (#39).
const ReportFormat = "datum-report/1"

// JSONOutput switches Check/Fetch from colorized human-readable text to a single JSON document
// on stdout, set by the CLI's --json flag before calling Check/Fetch.
var JSONOutput bool

// Status is the final, machine-readable outcome of processing a single dataset. It's populated
// alongside (not instead of) the human-readable text lines written to the per-dataset io.Writer
// in checkOneDataset/fetchOneDataset, so --json and the default colorized output always describe
// the same outcome.
type Status string

const (
	StatusOK      Status = "ok"      // remote unchanged, nothing to do
	StatusUpdated Status = "updated" // "update" policy: remote changed and was re-fetched
	StatusFetched Status = "fetched" // fetch command: dataset downloaded
	StatusStale   Status = "stale"   // "log" policy: remote changed, reported only
	StatusFail    Status = "fail"    // "fail" policy: remote changed, treated as an error
	StatusWarn    Status = "warn"    // unrecognized policy name in config
	StatusError   Status = "error"   // fingerprint/fetch failed against every configured source
	StatusDeleted Status = "deleted" // `datum delete` removed this dataset; check/fetch skipped it
)

// Result is one dataset's outcome. LockFingerprint/RemoteFingerprint are only set once a
// fingerprint has actually been computed (empty for e.g. StatusError from a failed fingerprint).
type Result struct {
	ID                string   `json:"id"`
	Status            Status   `json:"status"`
	Message           string   `json:"message,omitempty"`
	LockFingerprint   string   `json:"lock_fingerprint,omitempty"`
	RemoteFingerprint string   `json:"remote_fingerprint,omitempty"`
	Warnings          []string `json:"warnings,omitempty"`

	// Staged is set by a staged fetch (--stage-dir): what was written for this dataset and
	// what a promote step would remove.
	Staged *StagedResult `json:"staged,omitempty"`
}

// StagedResult describes one dataset's staged fetch output (#38). Paths are slash-separated:
// Target relative to the project, Files and Removed relative to the target.
type StagedResult struct {
	Target string `json:"target"`
	// Path is where the staged copy of the target was written.
	Path string `json:"path"`
	// Kind is "file" for a single-file target and "dir" for a directory.
	Kind   string `json:"kind"`
	SHA256 string `json:"sha256,omitempty"`
	Size   int64  `json:"size,omitempty"`
	// Files lists every file written under a directory target.
	Files []FileEntry `json:"files,omitempty"`
	// Removed lists the files the previous fetch wrote that this one no longer has.
	Removed []string `json:"removed,omitempty"`
}

// Report is the single top-level JSON document Check/Fetch print to stdout when JSONOutput is set.
type Report struct {
	Format         string   `json:"format"`
	Results        []Result `json:"results"`
	LockWriteError string   `json:"lock_write_error,omitempty"`
}

// reportError prints a top-level failure - one that happens before any dataset is processed, e.g.
// a config or lockfile parse error - as plain text or, under JSONOutput, as a single JSON object,
// and returns the exit code (2) both callers use for this case.
func reportError(prefix string, err error) int {
	msg := redact(fmt.Sprintf("%s: %v", prefix, err))
	if JSONOutput {
		data, jsonErr := json.MarshalIndent(errorDocument(msg), "", "  ")
		if jsonErr == nil {
			fmt.Println(string(data))
			return 2
		}
	}
	fmt.Println(msg)
	return 2
}

// printReport prints the final per-dataset results as a single JSON document.
func printReport(results []Result, lockErr error) {
	if results == nil {
		results = []Result{}
	}
	redactResults(results)
	r := Report{Format: ReportFormat, Results: results}
	if lockErr != nil {
		r.LockWriteError = redact(lockErr.Error())
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		fmt.Printf("json encode error: %v\n", err)
		return
	}
	fmt.Println(string(data))
}

// errorJSON is the document printed for a failure before any dataset is processed.
type errorJSON struct {
	Format string `json:"format"`
	Error  string `json:"error"`
}

func errorDocument(msg string) errorJSON {
	return errorJSON{Format: ReportFormat, Error: msg}
}
