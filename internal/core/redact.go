package core

import (
	"regexp"
	"sort"
	"strings"
	"sync"
)

// Redacted replaces secret text in datum's output.
const Redacted = "[redacted]"

// minSecretLen is the shortest substituted value that gets redacted. Shorter values (a "1" or
// "v2" flag) would turn unrelated text into noise and aren't credentials.
const minSecretLen = 4

var (
	secretMu     sync.Mutex
	secretValues []string
)

// resetSecrets forgets the values remembered by the previous config load.
func resetSecrets() {
	secretMu.Lock()
	secretValues = nil
	secretMu.Unlock()
}

// rememberSecret records a value substituted for ${NAME}, so redact can hide it.
func rememberSecret(value string) {
	if len(value) < minSecretLen {
		return
	}
	secretMu.Lock()
	defer secretMu.Unlock()
	for _, v := range secretValues {
		if v == value {
			return
		}
	}
	secretValues = append(secretValues, value)
	// Longest first, so a value that contains another is replaced whole.
	sort.Slice(secretValues, func(i, j int) bool { return len(secretValues[i]) > len(secretValues[j]) })
}

var (
	urlPattern      = regexp.MustCompile(`(?i)\b[a-z][a-z0-9+.-]*://[^\s"'<>]+`)
	userinfoPattern = regexp.MustCompile(`^([A-Za-z][A-Za-z0-9+.-]*://)[^/?#@]*@`)
	queryPattern    = regexp.MustCompile(`([?&][^=&#]*=)[^&#]*`)
)

// redact hides secrets in a message before datum prints or stores it (#42):
//   - every value substituted for a ${NAME} reference in this run;
//   - the user:password@ part of any URL;
//   - the value of every URL query parameter (the name is kept).
//
// It's applied to result messages and warnings, top-level errors, text output and the lockfile's
// inaccessible_error, never to fingerprints or hashes.
func redact(s string) string {
	if s == "" {
		return s
	}
	secretMu.Lock()
	values := secretValues
	secretMu.Unlock()
	for _, v := range values {
		s = strings.ReplaceAll(s, v, Redacted)
	}
	return urlPattern.ReplaceAllStringFunc(s, func(u string) string {
		u = userinfoPattern.ReplaceAllString(u, "${1}"+Redacted+"@")
		if q := strings.IndexByte(u, '?'); q >= 0 {
			u = u[:q] + queryPattern.ReplaceAllString(u[q:], "${1}"+Redacted)
		}
		return u
	})
}

func redactAll(values []string) []string {
	for i := range values {
		values[i] = redact(values[i])
	}
	return values
}

// redactResults hides secrets in every message and warning of a report.
func redactResults(results []Result) {
	for i := range results {
		results[i].Message = redact(results[i].Message)
		results[i].Warnings = redactAll(results[i].Warnings)
	}
}
