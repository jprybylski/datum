package core

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"reflect"
	"runtime"
	"strings"
	"testing"

	"github.com/jprybylski/datum/internal/registry"
)

func TestReplaceGitIgnoreBlock(t *testing.T) {
	existing := []byte("vendor/\n\n" + gitIgnoreBegin + "\n/old.csv\n" + gitIgnoreEnd + "\n")
	got, err := replaceGitIgnoreBlock(existing, []string{"/data/b.csv", "/data/a.csv"})
	if err != nil {
		t.Fatal(err)
	}
	want := "vendor/\n\n" + gitIgnoreBegin + "\n/data/a.csv\n/data/b.csv\n" + gitIgnoreEnd + "\n"
	if string(got) != want {
		t.Fatalf("ignore content:\n%s\nwant:\n%s", got, want)
	}
	got, err = replaceGitIgnoreBlock(got, nil)
	if err != nil || string(got) != "vendor/\n" {
		t.Fatalf("removed block = %q, %v", got, err)
	}
}

func TestReplaceGitIgnoreBlockFormattingAndErrors(t *testing.T) {
	t.Run("CRLF and duplicate entries", func(t *testing.T) {
		got, err := replaceGitIgnoreBlock([]byte("vendor/\r\n"), []string{"/b", "/a", "/a"})
		if err != nil {
			t.Fatal(err)
		}
		want := "vendor/\r\n\r\n" + gitIgnoreBegin + "\r\n/a\r\n/b\r\n" + gitIgnoreEnd + "\r\n"
		if string(got) != want {
			t.Fatalf("replaceGitIgnoreBlock() = %q, want %q", got, want)
		}
	})

	for _, test := range []struct {
		name    string
		content string
	}{
		{"duplicate begin", gitIgnoreBegin + "\n" + gitIgnoreBegin + "\n" + gitIgnoreEnd + "\n"},
		{"duplicate end", gitIgnoreBegin + "\n" + gitIgnoreEnd + "\n" + gitIgnoreEnd + "\n"},
		{"missing end", gitIgnoreBegin + "\n"},
		{"end before begin", gitIgnoreEnd + "\n" + gitIgnoreBegin + "\n"},
	} {
		t.Run(test.name, func(t *testing.T) {
			if _, err := replaceGitIgnoreBlock([]byte(test.content), []string{"/data"}); err == nil {
				t.Fatal("replaceGitIgnoreBlock() succeeded for malformed block")
			}
		})
	}
}

func TestGitIgnorePatternEscapesMetacharacters(t *testing.T) {
	if got, want := gitIgnorePattern("data/a [final]*.csv"), `/data/a\ \[final\]\*.csv`; got != want {
		t.Fatalf("gitIgnorePattern() = %q, want %q", got, want)
	}
}

func TestReconcileIgnoresOutsideWorkingCopyIsNoop(t *testing.T) {
	enterTempDir(t)
	original := runVCSCommand
	t.Cleanup(func() { runVCSCommand = original })
	runVCSCommand = func(string, ...string) ([]byte, error) {
		t.Fatal("VCS command called outside a working copy")
		return nil, nil
	}
	if err := reconcileIgnores(ignoreTestConfig(true)); err != nil {
		t.Fatal(err)
	}
}

func TestPrepareIgnorePlanDetectsSVNAndPropagatesErrors(t *testing.T) {
	dir := enterTempDir(t)
	if err := os.Mkdir(filepath.Join(dir, ".svn"), 0o755); err != nil {
		t.Fatal(err)
	}
	original := runVCSCommand
	t.Cleanup(func() { runVCSCommand = original })
	runVCSCommand = func(string, ...string) ([]byte, error) {
		return []byte(`<?xml version="1.0"?><properties></properties>`), nil
	}
	plan, err := prepareIgnorePlan(ignoreTestConfig(false))
	if err != nil {
		t.Fatal(err)
	}
	if plan.git != nil || plan.svn == nil {
		t.Fatalf("detected plan = %#v", plan)
	}

	runVCSCommand = func(string, ...string) ([]byte, error) { return nil, errors.New("svn unavailable") }
	if _, err := prepareIgnorePlan(ignoreTestConfig(false)); err == nil || !strings.Contains(err.Error(), "svn unavailable") {
		t.Fatalf("prepareIgnorePlan(SVN error) = %v", err)
	}
}

func TestPrepareIgnorePlanGetwdError(t *testing.T) {
	original := getWorkingDirectory
	t.Cleanup(func() { getWorkingDirectory = original })
	getWorkingDirectory = func() (string, error) { return "", errors.New("getwd failed") }
	if _, err := prepareIgnorePlan(ignoreTestConfig(false)); err == nil || !strings.Contains(err.Error(), "getwd failed") {
		t.Fatalf("prepareIgnorePlan(getwd error) = %v", err)
	}
}

func TestRunVCSCommandReportsCommandErrors(t *testing.T) {
	if _, err := runVCSCommand(filepath.Join(t.TempDir(), "missing-vcs-command")); err == nil {
		t.Fatal("runVCSCommand() hid an execution error")
	}
}

func TestReconcileGitIgnore(t *testing.T) {
	dir := enterTempDir(t)
	runCommand(t, "git", "init", "--quiet", dir)
	if err := os.WriteFile(filepath.Join(dir, ".gitignore"), []byte("vendor/\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	cfg := ignoreTestConfig(true)
	if err := reconcileIgnores(cfg); err != nil {
		t.Fatal(err)
	}
	content, err := os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(string(content), "vendor/\n") || !strings.Contains(string(content), "/data/example.csv") {
		t.Fatalf(".gitignore = %q", content)
	}
	if err := reconcileIgnores(cfg); err != nil {
		t.Fatal(err)
	}
	cfg.Defaults.Ignore = false
	if err := reconcileIgnores(cfg); err != nil {
		t.Fatal(err)
	}
	content, err = os.ReadFile(filepath.Join(dir, ".gitignore"))
	if err != nil || string(content) != "vendor/\n" {
		t.Fatalf("cleaned .gitignore = %q, %v", content, err)
	}
}

func TestReconcileGitIgnoreRejectsTrackedTarget(t *testing.T) {
	dir := enterTempDir(t)
	runCommand(t, "git", "init", "--quiet", dir)
	if err := os.MkdirAll(filepath.Join(dir, "data"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "data", "example.csv"), []byte("tracked"), 0o644); err != nil {
		t.Fatal(err)
	}
	runCommand(t, "git", "-C", dir, "add", "data/example.csv")
	if err := reconcileIgnores(ignoreTestConfig(true)); err == nil || !strings.Contains(err.Error(), "already tracked by Git") {
		t.Fatalf("reconcile error = %v", err)
	}
}

func TestRelativeTargetValidation(t *testing.T) {
	root := t.TempDir()
	cwd := filepath.Join(root, "project")
	if err := os.MkdirAll(cwd, 0o755); err != nil {
		t.Fatal(err)
	}
	if got, err := relativeTarget(root, cwd, Dataset{ID: "ok", Target: "data/file.csv"}); err != nil || got != filepath.Join("project", "data", "file.csv") {
		t.Fatalf("relativeTarget(valid) = %q, %v", got, err)
	}
	for _, target := range []string{root, filepath.Join(root, "..", "outside.csv"), "data/bad\nname.csv"} {
		if _, err := relativeTarget(root, cwd, Dataset{ID: "bad", Target: target}); err == nil {
			t.Fatalf("relativeTarget(%q) succeeded", target)
		}
	}
}

func TestPrepareGitIgnoreFailuresAndMode(t *testing.T) {
	dir := t.TempDir()
	original := runVCSCommand
	t.Cleanup(func() { runVCSCommand = original })

	runVCSCommand = func(string, ...string) ([]byte, error) { return nil, errors.New("git unavailable") }
	if _, err := prepareGitIgnore(dir, dir, ignoreTestConfig(true)); err == nil || !strings.Contains(err.Error(), "check whether") {
		t.Fatalf("prepareGitIgnore(command failure) = %v", err)
	}

	runVCSCommand = func(string, ...string) ([]byte, error) { return nil, nil }
	ignorePath := filepath.Join(dir, ".gitignore")
	if err := os.WriteFile(ignorePath, []byte("vendor/\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	plan, err := prepareGitIgnore(dir, dir, ignoreTestConfig(true))
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && plan.mode != 0o600 {
		t.Fatalf("gitignore mode = %o, want 600", plan.mode)
	}
	if err := os.WriteFile(ignorePath, []byte(gitIgnoreBegin+"\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := prepareGitIgnore(dir, dir, ignoreTestConfig(true)); err == nil {
		t.Fatal("prepareGitIgnore() accepted malformed managed block")
	}

	if err := os.Remove(ignorePath); err != nil {
		t.Fatal(err)
	}
	if _, err := prepareGitIgnore(dir, dir, ignoreTestConfig(false)); err != nil {
		t.Fatalf("prepareGitIgnore(no entries) = %v", err)
	}
	outside := ignoreTestConfig(true)
	outside.Datasets[0].Target = filepath.Join(dir, "..", "outside.csv")
	if _, err := prepareGitIgnore(dir, dir, outside); err == nil || !strings.Contains(err.Error(), "outside the version-control root") {
		t.Fatalf("prepareGitIgnore(outside target) = %v", err)
	}

	if err := os.Mkdir(ignorePath, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, err := prepareGitIgnore(dir, dir, ignoreTestConfig(false)); err == nil {
		t.Fatal("prepareGitIgnore() read a directory as .gitignore")
	}
}

func TestGitIgnorePlanApplyNoopsAndWrites(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".gitignore")
	if err := (&gitIgnorePlan{path: path, mode: 0o644}).apply(); err != nil {
		t.Fatalf("empty missing plan: %v", err)
	}
	if err := os.WriteFile(path, []byte("same\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := (&gitIgnorePlan{path: path, content: []byte("same\n"), mode: 0o644}).apply(); err != nil {
		t.Fatalf("unchanged plan: %v", err)
	}
	if err := (&gitIgnorePlan{path: path, content: []byte("new\n"), mode: 0o600}).apply(); err != nil {
		t.Fatalf("changed plan: %v", err)
	}
	if got, err := os.ReadFile(path); err != nil || string(got) != "new\n" {
		t.Fatalf("written content = %q, %v", got, err)
	}
}

func TestPrepareAndApplySVNIgnore(t *testing.T) {
	dir := enterTempDir(t)
	dataDir := filepath.Join(dir, "data")
	if err := os.MkdirAll(dataDir, 0o755); err != nil {
		t.Fatal(err)
	}
	original := runVCSCommand
	t.Cleanup(func() { runVCSCommand = original })
	written := map[string]string{}
	runVCSCommand = func(name string, args ...string) ([]byte, error) {
		if name != "svn" {
			t.Fatalf("command = %s, want svn", name)
		}
		switch args[0] {
		case "propget":
			return []byte("<?xml version=\"1.0\"?><properties></properties>"), nil
		case "info":
			if filepath.Clean(args[1]) == dataDir {
				return []byte("versioned parent"), nil
			}
			return nil, errors.New("unversioned target")
		case "propset":
			content, err := os.ReadFile(args[3])
			if err != nil {
				t.Fatal(err)
			}
			written[args[1]] = string(content)
			return nil, nil
		default:
			t.Fatalf("unexpected svn args: %v", args)
			return nil, nil
		}
	}
	plan, err := prepareSVNIgnore(dir, dir, ignoreTestConfig(true))
	if err != nil {
		t.Fatal(err)
	}
	if err := plan.apply(); err != nil {
		t.Fatal(err)
	}
	if written["svn:ignore"] != "example.csv\n" || written[svnOwnedProp] != "example.csv\n" {
		t.Fatalf("written SVN properties = %#v", written)
	}
}

func TestSVNPropertyMap(t *testing.T) {
	dir := t.TempDir()
	original := runVCSCommand
	t.Cleanup(func() { runVCSCommand = original })

	t.Run("parses matching relative and absolute targets", func(t *testing.T) {
		absolute := filepath.Join(dir, "absolute")
		runVCSCommand = func(string, ...string) ([]byte, error) {
			return []byte(`<?xml version="1.0"?><properties>` +
				`<target path="relative"><property name="svn:ignore">b&#13;&#10;a&#10;a&#10;</property><property name="other">skip</property></target>` +
				`<target path="` + absolute + `"><property name="svn:ignore">c</property></target></properties>`), nil
		}
		got, err := svnPropertyMap(dir, "svn:ignore", true)
		if err != nil {
			t.Fatal(err)
		}
		if !reflect.DeepEqual(got[filepath.Join(dir, "relative")], []string{"a", "b"}) || !reflect.DeepEqual(got[absolute], []string{"c"}) {
			t.Fatalf("svnPropertyMap() = %#v", got)
		}
	})

	t.Run("missing property in valid working copy", func(t *testing.T) {
		calls := 0
		runVCSCommand = func(_ string, args ...string) ([]byte, error) {
			calls++
			if args[0] == "info" {
				return []byte("ok"), nil
			}
			return nil, errors.New("property absent")
		}
		got, err := svnPropertyMap(dir, "svn:ignore", false)
		if err != nil || len(got) != 0 || calls != 2 {
			t.Fatalf("svnPropertyMap(absent) = %#v, %v, calls=%d", got, err, calls)
		}
	})

	t.Run("invalid working copy", func(t *testing.T) {
		runVCSCommand = func(string, ...string) ([]byte, error) { return nil, errors.New("not svn") }
		if _, err := svnPropertyMap(dir, "svn:ignore", false); err == nil {
			t.Fatal("svnPropertyMap() hid invalid working copy")
		}
	})

	t.Run("invalid XML", func(t *testing.T) {
		runVCSCommand = func(string, ...string) ([]byte, error) { return []byte("<broken>"), nil }
		if _, err := svnPropertyMap(dir, "svn:ignore", false); err == nil || !strings.Contains(err.Error(), "parse SVN") {
			t.Fatalf("svnPropertyMap(invalid XML) = %v", err)
		}
	})
}

func TestPrepareSVNIgnoreReconcilesOwnedAndUserEntries(t *testing.T) {
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	original := runVCSCommand
	t.Cleanup(func() { runVCSCommand = original })
	runVCSCommand = func(_ string, args ...string) ([]byte, error) {
		switch args[0] {
		case "info":
			if filepath.Clean(args[1]) == dataDir {
				return []byte("versioned"), nil
			}
			return nil, errors.New("unversioned")
		case "propget":
			property := args[1]
			value := ""
			if property == svnOwnedProp {
				value = "old.csv\nshared.csv\n"
			} else {
				value = "old.csv\nshared.csv\nuser.tmp\n"
			}
			return []byte(`<?xml version="1.0"?><properties><target path="` + dataDir + `"><property name="` + property + `">` + value + `</property></target></properties>`), nil
		default:
			return nil, errors.New("unexpected command")
		}
	}
	cfg := ignoreTestConfig(true)
	cfg.Datasets = append(cfg.Datasets, Dataset{ID: "shared", Target: "data/shared.csv", Ignore: boolPtr(true)})
	plan, err := prepareSVNIgnore(dir, dir, cfg)
	if err != nil {
		t.Fatal(err)
	}
	if len(plan.updates) != 1 {
		t.Fatalf("updates = %#v", plan.updates)
	}
	update := plan.updates[0]
	if !reflect.DeepEqual(update.ignore, []string{"example.csv", "shared.csv", "user.tmp"}) || !reflect.DeepEqual(update.owned, []string{"example.csv", "shared.csv"}) {
		t.Fatalf("reconciled update = %#v", update)
	}
}

func TestPrepareSVNIgnoreRejectsUnsafeTargets(t *testing.T) {
	dir := t.TempDir()
	original := runVCSCommand
	t.Cleanup(func() { runVCSCommand = original })
	propertyXML := []byte(`<?xml version="1.0"?><properties></properties>`)

	for _, test := range []struct {
		name, target string
		info         func(string) error
		want         string
	}{
		{"wildcard", "data/*.csv", func(string) error { return nil }, "cannot be represented"},
		{"unversioned parent", "data/example.csv", func(string) error { return errors.New("not versioned") }, "not a versioned SVN directory"},
		{"tracked target", "data/example.csv", func(path string) error {
			if path == filepath.Join(dir, "data") {
				return nil
			}
			return nil
		}, "already tracked by SVN"},
	} {
		t.Run(test.name, func(t *testing.T) {
			runVCSCommand = func(_ string, args ...string) ([]byte, error) {
				if args[0] == "propget" {
					return propertyXML, nil
				}
				return nil, test.info(filepath.Clean(args[1]))
			}
			cfg := ignoreTestConfig(true)
			cfg.Datasets[0].Target = test.target
			if _, err := prepareSVNIgnore(dir, dir, cfg); err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("prepareSVNIgnore() = %v, want %q", err, test.want)
			}
		})
	}

	outside := ignoreTestConfig(true)
	outside.Datasets[0].Target = filepath.Join(dir, "..", "outside.csv")
	runVCSCommand = func(_ string, args ...string) ([]byte, error) {
		if args[0] == "propget" {
			return propertyXML, nil
		}
		return nil, errors.New("not versioned")
	}
	if _, err := prepareSVNIgnore(dir, dir, outside); err == nil || !strings.Contains(err.Error(), "outside the version-control root") {
		t.Fatalf("prepareSVNIgnore(outside target) = %v", err)
	}
}

func TestPrepareSVNIgnoreCurrentPropertyError(t *testing.T) {
	dir := t.TempDir()
	dataDir := filepath.Join(dir, "data")
	infoCalls := 0
	original := runVCSCommand
	t.Cleanup(func() { runVCSCommand = original })
	runVCSCommand = func(_ string, args ...string) ([]byte, error) {
		switch args[0] {
		case "propget":
			if args[1] == svnOwnedProp {
				return []byte(`<?xml version="1.0"?><properties></properties>`), nil
			}
			return nil, errors.New("property read failed")
		case "info":
			infoCalls++
			if infoCalls == 1 && filepath.Clean(args[1]) == dataDir {
				return []byte("versioned"), nil
			}
			if infoCalls == 3 {
				return nil, errors.New("property read failed")
			}
			return nil, errors.New("not versioned")
		default:
			return nil, errors.New("unexpected command")
		}
	}
	if _, err := prepareSVNIgnore(dir, dir, ignoreTestConfig(true)); err == nil || !strings.Contains(err.Error(), "property read failed") {
		t.Fatalf("prepareSVNIgnore(property error) = %v", err)
	}
}

func TestSetSVNPropertyAndPlanErrors(t *testing.T) {
	dir := t.TempDir()
	original := runVCSCommand
	t.Cleanup(func() { runVCSCommand = original })
	calls := make([]string, 0)
	runVCSCommand = func(_ string, args ...string) ([]byte, error) {
		calls = append(calls, strings.Join(args, " "))
		return nil, nil
	}
	if err := setSVNProperty(dir, "svn:ignore", nil, false); err != nil || len(calls) != 0 {
		t.Fatalf("absent empty property = %v, calls=%v", err, calls)
	}
	if err := setSVNProperty(dir, "svn:ignore", nil, true); err != nil || len(calls) != 1 || !strings.HasPrefix(calls[0], "propdel svn:ignore") {
		t.Fatalf("property deletion = %v, calls=%v", err, calls)
	}
	if err := setSVNProperty(dir, "svn:ignore", []string{"b", "a"}, false); err != nil || len(calls) != 2 || !strings.HasPrefix(calls[1], "propset svn:ignore --file") {
		t.Fatalf("property set = %v, calls=%v", err, calls)
	}

	runVCSCommand = func(_ string, args ...string) ([]byte, error) {
		if args[1] == "svn:ignore" {
			return nil, errors.New("ignore failed")
		}
		return nil, nil
	}
	plan := &svnIgnorePlan{updates: []svnPropertyUpdate{{dir: dir, ignore: []string{"file"}, owned: []string{"file"}}}}
	if err := plan.apply(); err == nil || !strings.Contains(err.Error(), "ignore failed") {
		t.Fatalf("plan.apply(first property) = %v", err)
	}
	runVCSCommand = func(_ string, args ...string) ([]byte, error) {
		if args[1] == svnOwnedProp {
			return nil, errors.New("owned failed")
		}
		return nil, nil
	}
	if err := plan.apply(); err == nil || !strings.Contains(err.Error(), "owned failed") {
		t.Fatalf("plan.apply(second property) = %v", err)
	}
}

func TestSetSVNPropertyTemporaryFileFailures(t *testing.T) {
	original := createSVNTemp
	t.Cleanup(func() { createSVNTemp = original })
	for _, test := range []struct {
		name      string
		createErr error
		writeErr  error
		closeErr  error
	}{
		{"create", errors.New("create failed"), nil, nil},
		{"write", nil, errors.New("write failed"), nil},
		{"close", nil, nil, errors.New("close failed")},
	} {
		t.Run(test.name, func(t *testing.T) {
			createSVNTemp = func() (svnTempFile, error) {
				if test.createErr != nil {
					return nil, test.createErr
				}
				return fakeSVNTempFile{writeErr: test.writeErr, closeErr: test.closeErr}, nil
			}
			if err := setSVNProperty(t.TempDir(), "svn:ignore", []string{"file"}, false); err == nil || !strings.Contains(err.Error(), test.name+" failed") {
				t.Fatalf("setSVNProperty() = %v", err)
			}
		})
	}
}

type fakeSVNTempFile struct {
	writeErr error
	closeErr error
}

func (f fakeSVNTempFile) WriteString(value string) (int, error) {
	if f.writeErr != nil {
		return 0, f.writeErr
	}
	return len(value), nil
}

func (f fakeSVNTempFile) Close() error { return f.closeErr }
func (f fakeSVNTempFile) Name() string { return "datum-test-temp" }

func TestApplyIgnorePlanWrapsErrors(t *testing.T) {
	dir := t.TempDir()
	blockingFile := filepath.Join(dir, "file")
	if err := os.WriteFile(blockingFile, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	plan := &ignorePlan{git: &gitIgnorePlan{path: filepath.Join(blockingFile, ".gitignore"), content: []byte("x"), mode: 0o644}}
	if err := applyIgnorePlan(plan); err == nil || !strings.Contains(err.Error(), "update Git") {
		t.Fatalf("applyIgnorePlan(git) = %v", err)
	}

	original := runVCSCommand
	t.Cleanup(func() { runVCSCommand = original })
	runVCSCommand = func(string, ...string) ([]byte, error) { return nil, errors.New("svn failed") }
	plan = &ignorePlan{svn: &svnIgnorePlan{updates: []svnPropertyUpdate{{dir: dir, ignore: []string{"x"}}}}}
	if err := applyIgnorePlan(plan); err == nil || !strings.Contains(err.Error(), "update SVN") {
		t.Fatalf("applyIgnorePlan(svn) = %v", err)
	}
}

func TestAtomicWriteErrors(t *testing.T) {
	dir := t.TempDir()
	blocking := filepath.Join(dir, "blocking")
	if err := os.WriteFile(blocking, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := atomicWrite(filepath.Join(blocking, "nested"), []byte("x"), 0o644); err == nil {
		t.Fatal("atomicWrite() succeeded beneath a regular file")
	}

	path := filepath.Join(dir, "output")
	if err := os.Mkdir(path+".tmp", 0o755); err != nil {
		t.Fatal(err)
	}
	if err := atomicWrite(path, []byte("x"), 0o644); err == nil {
		t.Fatal("atomicWrite() wrote through a temporary directory")
	}
	if err := os.Remove(path + ".tmp"); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(path, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := atomicWrite(path, []byte("x"), 0o644); err == nil {
		t.Fatal("atomicWrite() renamed over a directory")
	}
}

func boolPtr(value bool) *bool { return &value }

func ignoreTestConfig(ignore bool) *Config {
	return &Config{
		Version: 1, Defaults: Defaults{Policy: "fail", Algo: "sha256", Ignore: ignore},
		Datasets: []Dataset{{
			ID: "example", Target: "data/example.csv",
			Source: registry.Source{Type: "http", URL: "https://example.com/data.csv"},
		}},
	}
}

func runCommand(t *testing.T, name string, args ...string) {
	t.Helper()
	cmd := exec.Command(name, args...)
	if output, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%s %v: %v: %s", name, args, err, output)
	}
}
