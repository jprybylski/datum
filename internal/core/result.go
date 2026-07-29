package core

import (
	"encoding/json"
	"fmt"
)

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
}

// Report is the single top-level JSON document Check/Fetch print to stdout when JSONOutput is set.
type Report struct {
	Results        []Result `json:"results"`
	LockWriteError string   `json:"lock_write_error,omitempty"`
}

// reportError prints a top-level failure - one that happens before any dataset is processed, e.g.
// a config or lockfile parse error - as plain text or, under JSONOutput, as a single JSON object,
// and returns the exit code (2) both callers use for this case.
func reportError(prefix string, err error) int {
	msg := fmt.Sprintf("%s: %v", prefix, err)
	if JSONOutput {
		data, jsonErr := json.MarshalIndent(struct {
			Error string `json:"error"`
		}{Error: msg}, "", "  ")
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
	r := Report{Results: results}
	if lockErr != nil {
		r.LockWriteError = lockErr.Error()
	}
	data, err := json.MarshalIndent(r, "", "  ")
	if err != nil {
		fmt.Printf("json encode error: %v\n", err)
		return
	}
	fmt.Println(string(data))
}
