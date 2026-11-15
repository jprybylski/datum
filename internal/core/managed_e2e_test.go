package core_test

// End-to-end tests for the opt-in flags for running datum on configs it didn't write (#35-#44),
// against the real file handler. See docs/embedding.md.

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"gopkg.in/yaml.v3"

	"github.com/jprybylski/datum/internal/core"
	_ "github.com/jprybylski/datum/internal/handlers/file"
	_ "github.com/jprybylski/datum/internal/handlers/http"
)

// project is a temporary project folder with a config and lockfile.
type project struct {
	dir, cfg, lock string
}

func newProject(t *testing.T, config string) *project {
	t.Helper()
	dir := t.TempDir()
	p := &project{dir: dir, cfg: filepath.Join(dir, ".data.yaml"), lock: filepath.Join(dir, ".data.lock.yaml")}
	p.write(t, ".data.yaml", config)
	return p
}

func (p *project) write(t *testing.T, rel, content string) {
	t.Helper()
	mustWriteFile(t, filepath.Join(p.dir, filepath.FromSlash(rel)), content)
}

func (p *project) read(t *testing.T, rel string) string {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(p.dir, filepath.FromSlash(rel)))
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// tree maps every file under dir (slash-separated relative path) to its content.
func tree(t *testing.T, dir string) map[string]string {
	t.Helper()
	files := map[string]string{}
	err := filepath.WalkDir(dir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		b, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		rel, _ := filepath.Rel(dir, path)
		files[filepath.ToSlash(rel)] = string(b)
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func assertSameTree(t *testing.T, label string, got, want map[string]string) {
	t.Helper()
	for path, content := range want {
		if got[path] != content {
			t.Errorf("%s: %s changed or missing", label, path)
		}
	}
	for path := range got {
		if _, ok := want[path]; !ok {
			t.Errorf("%s: unexpected new file %s", label, path)
		}
	}
}

// capture runs fn with os.Stdout redirected and returns what it printed and fn's exit code.
func capture(t *testing.T, fn func() int) (string, int) {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatal(err)
	}
	orig := os.Stdout
	os.Stdout = w
	done := make(chan []byte)
	go func() {
		b, _ := io.ReadAll(r)
		done <- b
	}()
	code := fn()
	os.Stdout = orig
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return string(<-done), code
}

// captureJSON is capture under --json, decoding the one document printed.
func captureJSON(t *testing.T, fn func() int) (map[string]any, int) {
	t.Helper()
	core.JSONOutput = true
	defer func() { core.JSONOutput = false }()
	out, code := capture(t, fn)
	var doc map[string]any
	if err := json.Unmarshal([]byte(out), &doc); err != nil {
		t.Fatalf("output is not one JSON document: %v\n%s", err, out)
	}
	if doc["format"] != core.ReportFormat {
		t.Errorf("format = %v, want %s", doc["format"], core.ReportFormat)
	}
	return doc, code
}

func results(t *testing.T, doc map[string]any) map[string]map[string]any {
	t.Helper()
	list, ok := doc["results"].([]any)
	if !ok {
		t.Fatalf("no results array: %v", doc)
	}
	byID := map[string]map[string]any{}
	for _, r := range list {
		m := r.(map[string]any)
		byID[m["id"].(string)] = m
	}
	return byID
}

func ctx() context.Context { return context.Background() }

// --- #35 --no-vcs -------------------------------------------------------------------------------

func TestNoVCS_NeverRunsGitOrSVNNorEditsIgnoreFiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("fake git/svn are shell scripts")
	}
	p := newProject(t, `version: 1
datasets:
  - id: pk
    desc: x
    source: {type: file, path: upstream/pk.csv}
    target: data/pk.csv
    ignore: true
`)
	p.write(t, "upstream/pk.csv", "a\n1\n")
	p.write(t, ".gitignore", "keep\n")
	for _, marker := range []string{".git", ".svn"} {
		if err := os.Mkdir(filepath.Join(p.dir, marker), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	bin := t.TempDir()
	sentinel := filepath.Join(t.TempDir(), "vcs-ran")
	for _, tool := range []string{"git", "svn"} {
		script := "#!/bin/sh\necho \"" + tool + " $*\" >> '" + sentinel + "'\nexit 1\n"
		if err := os.WriteFile(filepath.Join(bin, tool), []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	t.Setenv("PATH", bin)
	t.Chdir(p.dir)

	// Without the flag datum runs the (failing) fake git: the test would catch a regression.
	if code := core.Fetch(ctx(), p.cfg, p.lock, nil, 1); code != 2 {
		t.Fatalf("Fetch() without --no-vcs = %d, want 2 (fake git fails)", code)
	}
	if _, err := os.Stat(sentinel); err != nil {
		t.Fatalf("fake git was not run without --no-vcs: %v", err)
	}
	if err := os.Remove(sentinel); err != nil {
		t.Fatal(err)
	}

	opts := core.Options{NoVCS: true}
	doc, code := captureJSON(t, func() int { return core.FetchWith(ctx(), p.cfg, p.lock, nil, opts) })
	if code != 0 {
		t.Fatalf("FetchWith(--no-vcs) = %d, want 0: %v", code, doc)
	}
	warnings, _ := results(t, doc)["pk"]["warnings"].([]any)
	if len(warnings) != 1 || !strings.Contains(warnings[0].(string), "--no-vcs") {
		t.Errorf("warnings = %v, want the --no-vcs ignore warning", warnings)
	}
	if code := core.CheckWith(ctx(), p.cfg, p.lock, opts); code != 0 {
		t.Fatalf("CheckWith(--no-vcs) = %d, want 0", code)
	}
	if code := core.Init(filepath.Join(p.dir, "new.yaml"), core.InitOptions{Empty: true, Ignore: true, IgnoreSet: true, NoVCS: true}, strings.NewReader(""), io.Discard, false); code != 0 {
		t.Fatalf("Init(--no-vcs) = %d, want 0", code)
	}
	if _, err := os.Stat(sentinel); err == nil {
		t.Errorf("git or svn ran under --no-vcs: %s", p.readAbs(t, sentinel))
	}
	if got := p.read(t, ".gitignore"); got != "keep\n" {
		t.Errorf(".gitignore = %q, want it unchanged", got)
	}
}

func (p *project) readAbs(t *testing.T, path string) string {
	t.Helper()
	b, _ := os.ReadFile(path)
	return string(b)
}

func TestNoVCS_FromEnvironment(t *testing.T) {
	t.Setenv("DATUM_NO_VCS", "1")
	t.Setenv("DATUM_ALLOW_TYPES", "http, file")
	o := core.OptionsFromEnv()
	if !o.NoVCS || strings.Join(o.AllowTypes, ",") != "http,file" {
		t.Errorf("OptionsFromEnv() = %+v", o)
	}
}

// --- #36 --allow-types ---------------------------------------------------------------------------

func TestAllowTypes_RefusesOtherTypesBeforeAnyHandlerRuns(t *testing.T) {
	marker := filepath.Join(t.TempDir(), "ran")
	command := `{type: command, fingerprint_cmd: "touch '` + filepath.ToSlash(marker) + `'", fetch_cmd: "touch '` + filepath.ToSlash(marker) + `'"}`
	for name, datasets := range map[string]string{
		"source": `
  - id: ok
    desc: x
    source: {type: file, path: a.csv}
    target: data/a.csv
  - id: shell
    desc: x
    source: ` + command + `
    target: data/b`,
		"sources": `
  - id: shell
    desc: x
    sources:
      - {type: file, path: a.csv}
      - ` + command + `
    target: data/b`,
	} {
		t.Run(name, func(t *testing.T) {
			p := newProject(t, "version: 1\ndatasets:"+datasets+"\n")
			opts := core.Options{AllowTypes: []string{"http", "file", "git"}}
			for cmd, run := range map[string]func() int{
				"check": func() int { return core.CheckWith(ctx(), p.cfg, p.lock, opts) },
				"fetch": func() int { return core.FetchWith(ctx(), p.cfg, p.lock, nil, opts) },
				"audit": func() int { return core.AuditWith(p.cfg, p.lock, opts) },
			} {
				doc, code := captureJSON(t, run)
				msg, _ := doc["error"].(string)
				if code != 2 || !strings.Contains(msg, "(shell)") || !strings.Contains(msg, `"command" is not allowed`) {
					t.Errorf("%s: code %d, error %q; want 2 naming the dataset and type", cmd, code, msg)
				}
			}
			if _, err := os.Stat(marker); err == nil {
				t.Error("the command source ran")
			}
			if _, err := os.Stat(p.lock); err == nil {
				t.Error("a lockfile was written")
			}
		})
	}
}

// --- #37 --root / --target-prefix ----------------------------------------------------------------

func TestRoot_RefusesTargetsThatEscape(t *testing.T) {
	outside := t.TempDir()
	cases := map[string]string{
		"absolute":        filepath.ToSlash(filepath.Join(outside, "x.csv")),
		"dotdot":          "../x.csv",
		"inner dotdot":    "data/../../x.csv",
		"outside prefix":  "other/x.csv",
		"the lockfile":    ".data.lock.yaml",
		"the root itself": ".",
	}
	if runtime.GOOS != "windows" {
		cases["symlink escape"] = "data/link/x.csv"
	}
	for name, target := range cases {
		t.Run(name, func(t *testing.T) {
			p := newProject(t, `version: 1
datasets:
  - id: escapee
    desc: x
    source: {type: file, path: upstream/a.csv}
    target: "`+target+`"
`)
			p.write(t, "upstream/a.csv", "a\n")
			if err := os.MkdirAll(filepath.Join(p.dir, "data"), 0o755); err != nil {
				t.Fatal(err)
			}
			if runtime.GOOS != "windows" {
				if err := os.Symlink(outside, filepath.Join(p.dir, "data", "link")); err != nil {
					t.Fatal(err)
				}
			}
			opts := core.Options{Root: p.dir, TargetPrefix: "data"}
			if name == "the lockfile" || name == "the root itself" {
				opts.TargetPrefix = ""
			}
			doc, code := captureJSON(t, func() int { return core.FetchWith(ctx(), p.cfg, p.lock, nil, opts) })
			msg, _ := doc["error"].(string)
			if code != 2 || !strings.Contains(msg, "dataset 0 (escapee): target") {
				t.Errorf("code %d, error %q; want 2 naming the dataset", code, msg)
			}
			if entries, _ := os.ReadDir(outside); len(entries) != 0 {
				t.Errorf("something was written outside the root: %v", entries)
			}
		})
	}
}

func TestRoot_ResolvesTargetsAndFileSourcesFromTheRootNotTheCwd(t *testing.T) {
	p := newProject(t, `version: 1
datasets:
  - id: pk
    desc: x
    source: {type: file, path: upstream/pk.csv}
    target: data/pk.csv
`)
	p.write(t, "upstream/pk.csv", "a\n1\n")
	elsewhere := t.TempDir()
	t.Chdir(elsewhere)
	opts := core.Options{Root: p.dir, TargetPrefix: "data", NoVCS: true}
	if code := core.FetchWith(ctx(), p.cfg, p.lock, nil, opts); code != 0 {
		t.Fatalf("FetchWith(--root) = %d, want 0", code)
	}
	if got := p.read(t, "data/pk.csv"); got != "a\n1\n" {
		t.Errorf("target = %q", got)
	}
	if entries, _ := os.ReadDir(elsewhere); len(entries) != 0 {
		t.Errorf("wrote into the working directory: %v", entries)
	}
	doc, _ := captureJSON(t, func() int { return core.AuditWith(p.cfg, p.lock, opts) })
	entry := doc["entries"].([]any)[0].(map[string]any)
	if entry["target"] != "data/pk.csv" {
		t.Errorf("audit target = %v, want it as written", entry["target"])
	}
}

func TestLock_DirPathsCannotEscapeTheTarget(t *testing.T) {
	p := newProject(t, `version: 1
datasets:
  - id: tree
    desc: x
    source: {type: file, path: upstream}
    target: data/tree
`)
	p.write(t, "upstream/a.txt", "a")
	p.write(t, "victim.txt", "keep me")
	p.write(t, ".data.lock.yaml", `version: 1
items:
  tree:
    remote_fingerprint: old
    dir_paths: [a.txt, ../../victim.txt]
`)
	t.Chdir(p.dir)
	for name, run := range map[string]func() int{
		"fetch":  func() int { return core.Fetch(ctx(), p.cfg, p.lock, nil, 1) },
		"delete": func() int { return core.Delete(p.cfg, p.lock, []string{"tree"}, true, nil, io.Discard) },
	} {
		if _, code := capture(t, run); code != 2 {
			t.Errorf("%s = %d, want 2 (lock error)", name, code)
		}
	}
	if got := p.read(t, "victim.txt"); got != "keep me" {
		t.Errorf("victim.txt = %q", got)
	}
}

// --- #38 staged fetch ------------------------------------------------------------------------

const stagedConfig = `version: 1
datasets:
  - id: one
    desc: single file
    source: {type: file, path: upstream/one.csv}
    target: data/one.csv
  - id: left
    desc: shares data/tree
    source: {type: file, path: upstream/left}
    target: data/tree
  - id: right
    desc: shares data/tree
    source: {type: file, path: upstream/right}
    target: data/tree
  - id: own
    desc: owns its folder
    source: {type: file, path: upstream/own}
    target: data/own
`

func stagedProject(t *testing.T) *project {
	t.Helper()
	p := newProject(t, stagedConfig)
	p.write(t, "upstream/one.csv", "1\n")
	p.write(t, "upstream/left/a.txt", "a")
	p.write(t, "upstream/left/gone.txt", "gone")
	p.write(t, "upstream/right/b.txt", "b")
	p.write(t, "upstream/own/x.txt", "x")
	p.write(t, "upstream/own/sub/y.txt", "y")
	opts := core.Options{Root: p.dir, NoVCS: true}
	if code := core.FetchWith(ctx(), p.cfg, p.lock, nil, opts); code != 0 {
		t.Fatalf("initial fetch = %d", code)
	}
	p.write(t, "data/tree/foreign.txt", "not datum's")
	// Upstream changes: edit, add, remove.
	p.write(t, "upstream/one.csv", "2\n")
	p.write(t, "upstream/left/a.txt", "a2")
	if err := os.Remove(filepath.Join(p.dir, "upstream/left/gone.txt")); err != nil {
		t.Fatal(err)
	}
	p.write(t, "upstream/own/new.txt", "new")
	if err := os.Remove(filepath.Join(p.dir, "upstream/own/sub/y.txt")); err != nil {
		t.Fatal(err)
	}
	return p
}

func copyTree(t *testing.T, from, to string) {
	t.Helper()
	for rel, content := range tree(t, from) {
		mustWriteFile(t, filepath.Join(to, filepath.FromSlash(rel)), content)
	}
}

func lockItems(t *testing.T, path string) map[string]map[string]any {
	t.Helper()
	b, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var lk struct {
		Items map[string]map[string]any `yaml:"items"`
	}
	if err := yaml.Unmarshal(b, &lk); err != nil {
		t.Fatal(err)
	}
	for _, item := range lk.Items {
		delete(item, "checked_at")
	}
	return lk.Items
}

func TestStagedFetch_ChangesNothingAndMatchesAnInPlaceFetch(t *testing.T) {
	p := stagedProject(t)
	inPlace := t.TempDir()
	copyTree(t, p.dir, inPlace)
	before := tree(t, p.dir)

	stage := filepath.Join(t.TempDir(), "stage")
	lockOut := filepath.Join(t.TempDir(), "lock.yaml")
	opts := core.Options{Root: p.dir, StageDir: stage, LockOut: lockOut}
	doc, code := captureJSON(t, func() int { return core.FetchWith(ctx(), p.cfg, p.lock, nil, opts) })
	if code != 0 {
		t.Fatalf("staged fetch = %d: %v", code, doc)
	}
	assertSameTree(t, "project", tree(t, p.dir), before)

	// The same fetch in place, on a copy.
	q := &project{dir: inPlace, cfg: filepath.Join(inPlace, ".data.yaml"), lock: filepath.Join(inPlace, ".data.lock.yaml")}
	if code := core.FetchWith(ctx(), q.cfg, q.lock, nil, core.Options{Root: q.dir, NoVCS: true}); code != 0 {
		t.Fatalf("in-place fetch = %d", code)
	}
	staged, want := lockItems(t, lockOut), lockItems(t, q.lock)
	for id, item := range want {
		gotYAML, _ := yaml.Marshal(staged[id])
		wantYAML, _ := yaml.Marshal(item)
		if string(gotYAML) != string(wantYAML) {
			t.Errorf("lock item %s:\nstaged:   %s\nin place: %s", id, gotYAML, wantYAML)
		}
	}

	// Promoting the staged files gives the in-place tree.
	byID := results(t, doc)
	left := byID["left"]["staged"].(map[string]any)
	if removed, _ := left["removed"].([]any); len(removed) != 1 || removed[0] != "gone.txt" {
		t.Errorf("left removed = %v, want [gone.txt]", left["removed"])
	}
	own := byID["own"]["staged"].(map[string]any)
	if own["kind"] != "dir" || own["target"] != "data/own" {
		t.Errorf("own staged = %v", own)
	}
	one := byID["one"]["staged"].(map[string]any)
	if one["kind"] != "file" || one["size"].(float64) != 2 {
		t.Errorf("one staged = %v", one)
	}
	got := tree(t, stage)
	for rel, content := range tree(t, filepath.Join(inPlace, "data")) {
		if rel == "tree/foreign.txt" || rel == "tree/b.txt" {
			continue // not this fetch's to stage: foreign, or right's file (unchanged, restaged below)
		}
		if got["data/"+rel] != content {
			t.Errorf("stage data/%s = %q, want %q", rel, got["data/"+rel], content)
		}
	}
}

func TestStagedFetch_FlagChecks(t *testing.T) {
	p := newProject(t, `version: 1
datasets:
  - id: abs
    desc: x
    source: {type: file, path: a.csv}
    target: /tmp/abs.csv
`)
	for name, opts := range map[string]core.Options{
		"stage without lock-out": {StageDir: t.TempDir()},
		"lock-out is the lock":   {StageDir: t.TempDir(), LockOut: p.lock},
		"absolute target":        {StageDir: t.TempDir(), LockOut: filepath.Join(t.TempDir(), "l.yaml")},
		"stage dir is the root":  {Root: p.dir, StageDir: p.dir, LockOut: filepath.Join(t.TempDir(), "l.yaml")},
	} {
		if _, code := capture(t, func() int { return core.FetchWith(ctx(), p.cfg, p.lock, nil, opts) }); code != 2 {
			t.Errorf("%s: code %d, want 2", name, code)
		}
	}
	if _, code := capture(t, func() int {
		return core.CheckWith(ctx(), p.cfg, p.lock, core.Options{StageDir: "x", LockOut: "y"})
	}); code != 2 {
		t.Errorf("check with --stage-dir: code %d, want 2", code)
	}
}

func TestDirectorySync_ReplacesAnOwnedFolderWhole(t *testing.T) {
	p := stagedProject(t)
	opts := core.Options{Root: p.dir, NoVCS: true}
	if code := core.FetchWith(ctx(), p.cfg, p.lock, nil, opts); code != 0 {
		t.Fatalf("fetch = %d", code)
	}
	got := tree(t, filepath.Join(p.dir, "data"))
	want := map[string]string{
		"one.csv": "2\n", "own/x.txt": "x", "own/new.txt": "new",
		"tree/a.txt": "a2", "tree/b.txt": "b", "tree/foreign.txt": "not datum's",
	}
	assertSameTree(t, "data", got, want)
	if _, err := os.Stat(filepath.Join(p.dir, "data/own/sub")); err == nil {
		t.Error("data/own/sub should be gone with its only file")
	}
	entries, _ := os.ReadDir(filepath.Join(p.dir, "data"))
	for _, e := range entries {
		if strings.Contains(e.Name(), ".datum-") {
			t.Errorf("leftover %s", e.Name())
		}
	}
}

// --- #40 --report-only -----------------------------------------------------------------------

func TestReportOnly_WritesNothingUnderEveryPolicy(t *testing.T) {
	for policy, want := range map[string]string{"fail": "fail", "update": "stale", "log": "stale"} {
		t.Run(policy, func(t *testing.T) {
			p := newProject(t, `version: 1
datasets:
  - id: pk
    desc: x
    source: {type: file, path: upstream/pk.csv}
    target: data/pk.csv
    policy: `+policy+`
  - id: missing
    desc: x
    source: {type: file, path: upstream/pk.csv}
    target: data/missing.csv
    policy: `+policy+`
`)
			p.write(t, "upstream/pk.csv", "1\n")
			t.Chdir(p.dir)
			if code := core.FetchWith(ctx(), p.cfg, p.lock, nil, core.Options{NoVCS: true}); code != 0 {
				t.Fatalf("fetch = %d", code)
			}
			if err := os.Remove(filepath.Join(p.dir, "data/missing.csv")); err != nil {
				t.Fatal(err)
			}
			p.write(t, "upstream/pk.csv", "2\n")
			before := tree(t, p.dir)

			doc, _ := captureJSON(t, func() int {
				return core.CheckWith(ctx(), p.cfg, p.lock, core.Options{ReportOnly: true})
			})
			assertSameTree(t, "project", tree(t, p.dir), before)
			byID := results(t, doc)
			if got := byID["pk"]["status"]; got != want {
				t.Errorf("pk status = %v, want %s", got, want)
			}
			if policy == "update" {
				if msg, _ := byID["missing"]["message"].(string); !strings.Contains(msg, "would update") {
					t.Errorf("missing target message = %q", msg)
				}
			}
		})
	}
}

// --- #41 sizes and per-file hashes ---------------------------------------------------------------

func TestLock_RecordsSizesAndPerFileHashes_AndAuditReportsDrift(t *testing.T) {
	p := stagedProject(t)
	items := lockItems(t, p.lock)
	if items["one"]["size"] != 2 {
		t.Errorf("one size = %v, want 2", items["one"]["size"])
	}
	files, _ := items["own"]["dir_files"].([]any)
	if len(files) != 2 {
		t.Fatalf("own dir_files = %v, want 2 entries", items["own"]["dir_files"])
	}
	first := files[0].(map[string]any)
	if first["path"] != "sub/y.txt" && first["path"] != "x.txt" || first["size"] != 1 || len(first["sha256"].(string)) != 64 {
		t.Errorf("dir_files[0] = %v", first)
	}

	p.write(t, "data/own/x.txt", "edited")
	if err := os.Remove(filepath.Join(p.dir, "data/own/sub/y.txt")); err != nil {
		t.Fatal(err)
	}
	opts := core.Options{Root: p.dir, VerifyLocal: true}
	doc, code := captureJSON(t, func() int { return core.AuditWith(p.cfg, p.lock, opts) })
	if code != 0 {
		t.Fatalf("audit = %d", code)
	}
	entries := map[string]map[string]any{}
	for _, e := range doc["entries"].([]any) {
		m := e.(map[string]any)
		entries[m["id"].(string)] = m
		if m["status"] != "ok" {
			t.Errorf("%s status = %v; drift must not change the audit status", m["id"], m["status"])
		}
	}
	if entries["one"]["drift"] != "ok" || entries["own"]["drift"] != "changed" {
		t.Errorf("drift: one=%v own=%v", entries["one"]["drift"], entries["own"]["drift"])
	}
	perFile := map[string]string{}
	for _, f := range entries["own"]["files"].([]any) {
		m := f.(map[string]any)
		perFile[m["path"].(string)] = m["drift"].(string)
	}
	if perFile["x.txt"] != "changed" || perFile["sub/y.txt"] != "missing" {
		t.Errorf("per-file drift = %v", perFile)
	}

	// Without the flag, audit reads no local files and reports no drift, as before.
	doc, _ = captureJSON(t, func() int { return core.AuditWith(p.cfg, p.lock, core.Options{Root: p.dir}) })
	for _, e := range doc["entries"].([]any) {
		if _, ok := e.(map[string]any)["drift"]; ok {
			t.Error("drift reported without --verify-local")
		}
	}
}

func TestLock_FromDatum16StillLoads(t *testing.T) {
	p := newProject(t, `version: 1
datasets:
  - id: pk
    desc: x
    source: {type: file, path: upstream/pk.csv}
    target: data/pk.csv
`)
	p.write(t, "upstream/pk.csv", "1\n")
	p.write(t, "data/pk.csv", "1\n")
	p.write(t, ".data.lock.yaml", `version: 1
items:
  pk:
    local_sha256: 4355a46b19d348dc2f57c046f8ef63d4538ebb936000f3c9ee954a27460dd865
    remote_fingerprint: sha256:4355a46b19d348dc2f57c046f8ef63d4538ebb936000f3c9ee954a27460dd865
`)
	t.Chdir(p.dir)
	if code := core.CheckWith(ctx(), p.cfg, p.lock, core.Options{NoVCS: true}); code != 0 {
		t.Fatalf("check = %d, want 0", code)
	}
}

func TestAudit_EmptyConfigPrintsAnEmptyArray(t *testing.T) {
	p := newProject(t, "version: 1\ndatasets: []\n")
	out, code := capture(t, func() int {
		core.JSONOutput = true
		defer func() { core.JSONOutput = false }()
		return core.Audit(p.cfg, p.lock)
	})
	if code != 0 || !strings.Contains(out, `"entries": []`) {
		t.Errorf("audit = %d, %s; want an empty entries array, not null", code, out)
	}
}

// --- #42 secret hygiene ----------------------------------------------------------------------

const scopedConfig = `version: 1
datasets:
  - id: a
    desc: x
    source: {type: file, path: "${DATUM_TEST_A_DIR}/a.csv"}
    target: data/a.csv
  - id: b
    desc: x
    source:
      type: http
      url: "https://example.invalid/b.csv?token=${DATUM_TEST_B_TOKEN}"
      headers: {Authorization: "Bearer ${DATUM_TEST_B_TOKEN}"}
    target: data/${DATUM_TEST_B_DIR}/b.csv
`

func TestScopedSubstitution_OnlyTheSelectedDatasetsNeedTheirVariables(t *testing.T) {
	p := newProject(t, scopedConfig)
	p.write(t, "upstream/a.csv", "1\n")
	t.Setenv("DATUM_TEST_A_DIR", filepath.ToSlash(filepath.Join(p.dir, "upstream")))
	t.Chdir(p.dir)
	opts := core.Options{NoVCS: true}

	if code := core.FetchWith(ctx(), p.cfg, p.lock, []string{"a"}, opts); code != 0 {
		t.Fatalf("fetch a = %d, want 0 without b's variables", code)
	}
	if _, code := capture(t, func() int { return core.CheckWith(ctx(), p.cfg, p.lock, opts) }); code != 2 {
		t.Errorf("check = %d, want 2: check reads every dataset", code)
	}
	if _, code := capture(t, func() int { return core.FetchWith(ctx(), p.cfg, p.lock, []string{"b"}, opts) }); code != 2 {
		t.Errorf("fetch b = %d, want 2", code)
	}

	os.Unsetenv("DATUM_TEST_A_DIR")
	doc, code := captureJSON(t, func() int { return core.AuditWith(p.cfg, p.lock, opts) })
	if code != 0 {
		t.Fatalf("audit with no variables = %d, want 0: %v", code, doc)
	}
	if b := doc["entries"].([]any)[1].(map[string]any); b["target"] != "data/${DATUM_TEST_B_DIR}/b.csv" {
		t.Errorf("unresolved target shown as %v, want it as written", b["target"])
	}
	if _, code := capture(t, func() int { return core.Undelete(p.lock, []string{"a"}, io.Discard) }); code != 1 {
		t.Errorf("undelete = %d, want 1 (not deleted)", code)
	}
	t.Setenv("DATUM_TEST_A_DIR", "unused")
	if code := core.DeleteWith(p.cfg, p.lock, []string{"a"}, true, nil, io.Discard, opts); code != 0 {
		t.Errorf("delete a = %d, want 0 without b's variables", code)
	}
}

func TestEnvAllow_RefusesOtherReferencesBeforeAnyWork(t *testing.T) {
	p := newProject(t, scopedConfig)
	t.Setenv("DATUM_TEST_A_DIR", p.dir)
	for name, opts := range map[string]core.Options{
		"list":   {EnvAllow: []string{"DATUM_TEST_A_DIR"}},
		"prefix": {EnvPrefix: "DATUM_TEST_A_"},
	} {
		doc, code := captureJSON(t, func() int { return core.FetchWith(ctx(), p.cfg, p.lock, []string{"a"}, opts) })
		msg, _ := doc["error"].(string)
		if code != 2 || !strings.Contains(msg, `dataset 1 (b): field "source.url"`) || !strings.Contains(msg, "DATUM_TEST_B_TOKEN") {
			t.Errorf("%s: code %d, error %q", name, code, msg)
		}
	}
	if _, err := os.Stat(p.lock); err == nil {
		t.Error("a lockfile was written")
	}
	allowed := core.Options{EnvPrefix: "DATUM_TEST_", NoVCS: true}
	if _, code := capture(t, func() int { return core.AuditWith(p.cfg, p.lock, allowed) }); code != 0 {
		t.Errorf("audit with an allowing prefix = %d", code)
	}
}

func TestSecrets_NeverAppearInOutputOrLock(t *testing.T) {
	const secret = "s3cr3t-T0KEN-value"
	var seen []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.URL.RawQuery)
		http.Error(w, "nope", http.StatusNotFound)
	}))
	defer server.Close()
	p := newProject(t, `version: 1
datasets:
  - id: web
    desc: x
    source:
      type: http
      url: "`+server.URL+`/a.csv?token=${DATUM_TEST_SECRET}"
      headers: {X-Api-Key: "${DATUM_TEST_SECRET}"}
    target: data/web.csv
    policy: update
  - id: file
    desc: x
    source: {type: file, path: "missing/${DATUM_TEST_SECRET}.csv"}
    target: data/file.csv
    policy: update
`)
	t.Setenv("DATUM_TEST_SECRET", secret)
	t.Chdir(p.dir)
	opts := core.Options{NoVCS: true}

	var outputs []string
	for _, jsonMode := range []bool{false, true} {
		for _, run := range []func() int{
			func() int { return core.CheckWith(ctx(), p.cfg, p.lock, opts) },
			func() int { return core.FetchWith(ctx(), p.cfg, p.lock, nil, opts) },
			func() int { return core.AuditWith(p.cfg, p.lock, opts) },
		} {
			core.JSONOutput = jsonMode
			out, _ := capture(t, run)
			core.JSONOutput = false
			outputs = append(outputs, out)
		}
	}
	if len(seen) == 0 || !strings.Contains(seen[0], secret) {
		t.Fatalf("the server never got the substituted token (%v): the test proves nothing", seen)
	}
	lock := p.read(t, ".data.lock.yaml")
	if !strings.Contains(lock, "inaccessible_error") {
		t.Fatalf("lock has no error recorded:\n%s", lock)
	}
	for _, text := range append(outputs, lock) {
		if strings.Contains(text, secret) {
			t.Errorf("secret leaked:\n%s", text)
		}
	}
	if !strings.Contains(strings.Join(outputs, ""), core.Redacted) {
		t.Error("no redaction marker in any output")
	}
}

// --- #43/#44 URL restrictions at load ------------------------------------------------------------

func TestHTTPSOnly_RefusesPlainURLsAtLoad(t *testing.T) {
	for name, c := range map[string]struct {
		source string
		opts   core.Options
	}{
		"http source, --https-only":   {`{type: http, url: "http://example.com/a"}`, core.Options{HTTPSOnly: true}},
		"git source, --https-only":    {`{type: git, url: "http://example.com/r.git", ref: main, path: a}`, core.Options{HTTPSOnly: true}},
		"ssh git, --git-https-only":   {`{type: git, url: "ssh://git@example.com/r.git", ref: main, path: a}`, core.Options{GitHTTPSOnly: true}},
		"scp git, --git-https-only":   {`{type: git, url: "git@example.com:r.git", ref: main, path: a}`, core.Options{GitHTTPSOnly: true}},
		"git:// , --git-https-only":   {`{type: git, url: "git://example.com/r.git", ref: main, path: a}`, core.Options{GitHTTPSOnly: true}},
		"in sources, --https-only":    {"", core.Options{HTTPSOnly: true}},
		"file:// git, --https-only":   {`{type: git, url: "file:///srv/r.git", ref: main, path: a}`, core.Options{HTTPSOnly: true}},
		"mixed case http, https-only": {`{type: http, url: "HTTP://example.com/a"}`, core.Options{HTTPSOnly: true}},
	} {
		t.Run(name, func(t *testing.T) {
			dataset := "    source: " + c.source + "\n"
			if c.source == "" {
				dataset = "    sources:\n      - {type: http, url: \"https://example.com/a\"}\n      - {type: http, url: \"http://example.com/a\"}\n"
			}
			p := newProject(t, "version: 1\ndatasets:\n  - id: plain\n    desc: x\n"+dataset+"    target: data/a\n")
			doc, code := captureJSON(t, func() int { return core.CheckWith(ctx(), p.cfg, p.lock, c.opts) })
			msg, _ := doc["error"].(string)
			if code != 2 || !strings.Contains(msg, "dataset 0 (plain)") || !strings.Contains(msg, "only https://") {
				t.Errorf("code %d, error %q", code, msg)
			}
		})
	}
	// An https URL passes, and without the flags nothing is refused.
	p := newProject(t, "version: 1\ndatasets:\n  - id: ok\n    desc: x\n    source: {type: http, url: \"http://example.com/a\"}\n    target: data/a\n")
	if _, code := capture(t, func() int { return core.AuditWith(p.cfg, p.lock, core.Options{GitHTTPSOnly: true}) }); code != 0 {
		t.Errorf("--git-https-only refused an http source: %d", code)
	}
}

// --- #39 JSON for the lockfile commands ---------------------------------------------------------

func TestJSON_DeleteUndeleteUnlock(t *testing.T) {
	p := newProject(t, `version: 1
datasets:
  - id: pk
    desc: x
    source: {type: file, path: upstream/pk.csv}
    target: data/pk.csv
`)
	p.write(t, "upstream/pk.csv", "1\n")
	t.Chdir(p.dir)
	if code := core.FetchWith(ctx(), p.cfg, p.lock, nil, core.Options{NoVCS: true}); code != 0 {
		t.Fatal(code)
	}
	var out bytes.Buffer
	run := func(fn func() int) (map[string]any, int) {
		out.Reset()
		core.JSONOutput = true
		defer func() { core.JSONOutput = false }()
		code := fn()
		var doc map[string]any
		if err := json.Unmarshal(out.Bytes(), &doc); err != nil {
			t.Fatalf("not one JSON document: %v\n%s", err, out.String())
		}
		if doc["format"] != core.ReportFormat {
			t.Errorf("format = %v", doc["format"])
		}
		return doc, code
	}
	status := func(doc map[string]any) string {
		return doc["results"].([]any)[0].(map[string]any)["status"].(string)
	}

	if doc, code := run(func() int { return core.Delete(p.cfg, p.lock, []string{"pk"}, false, strings.NewReader("y\n"), &out) }); code != 2 || doc["error"] == nil {
		t.Errorf("delete --json without --yes = %d %v, want 2 and an error", code, doc)
	}
	if doc, code := run(func() int { return core.Delete(p.cfg, p.lock, []string{"pk"}, true, nil, &out) }); code != 0 || status(doc) != "deleted" {
		t.Errorf("delete = %d %v", code, doc)
	}
	if doc, code := run(func() int { return core.Delete(p.cfg, p.lock, []string{"pk"}, true, nil, &out) }); code != 0 || status(doc) != "skipped" {
		t.Errorf("delete again = %d %v", code, doc)
	}
	if doc, code := run(func() int { return core.Delete(p.cfg, p.lock, []string{"nope"}, true, nil, &out) }); code != 2 || doc["error"] == nil {
		t.Errorf("delete unknown = %d %v", code, doc)
	}
	if doc, code := run(func() int { return core.Undelete(p.lock, []string{"pk"}, &out) }); code != 0 || status(doc) != "undeleted" {
		t.Errorf("undelete = %d %v", code, doc)
	}
	if doc, code := run(func() int { return core.Undelete(p.lock, []string{"pk"}, &out) }); code != 1 || status(doc) != "error" {
		t.Errorf("undelete again = %d %v", code, doc)
	}
	if doc, code := run(func() int { return core.Unlock(p.cfg, p.lock, []string{"pk"}, true, nil, &out) }); code != 0 || status(doc) != "unlocked" {
		t.Errorf("unlock = %d %v", code, doc)
	}
	if doc, code := run(func() int { return core.Unlock(p.cfg, p.lock, []string{"pk"}, true, nil, &out) }); code != 1 || status(doc) != "error" {
		t.Errorf("unlock again = %d %v", code, doc)
	}
	if doc, code := run(func() int {
		return core.Init(filepath.Join(p.dir, "new.yaml"), core.InitOptions{ID: "x", Type: "file", Source: "a", Target: "b", NoVCS: true}, nil, &out, false)
	}); code != 0 || doc["datasets"].([]any)[0] != "x" {
		t.Errorf("init = %d %v", code, doc)
	}
	if doc, code := run(func() int {
		return core.Init(filepath.Join(p.dir, "new.yaml"), core.InitOptions{Empty: true}, nil, &out, false)
	}); code != 2 || !strings.Contains(doc["error"].(string), "already exists") {
		t.Errorf("init over an existing config = %d %v", code, doc)
	}
}
