package core

import (
	"bytes"
	"encoding/xml"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
)

const (
	gitIgnoreBegin = "# BEGIN datum managed datasets"
	gitIgnoreEnd   = "# END datum managed datasets"
	svnOwnedProp   = "datum:ignore"
)

type vcsCommand func(name string, args ...string) ([]byte, error)

var runVCSCommand vcsCommand = func(name string, args ...string) ([]byte, error) {
	cmd := exec.Command(name, args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		return output, fmt.Errorf("%s %s: %w: %s", name, strings.Join(args, " "), err, strings.TrimSpace(string(output)))
	}
	return output, nil
}

var getWorkingDirectory = os.Getwd

type ignorePlan struct {
	git *gitIgnorePlan
	svn *svnIgnorePlan
}

// reconcileIgnores updates ignore metadata only for version-control working copies that contain
// the invocation directory. Running Datum outside Git and SVN is intentionally a no-op.
func reconcileIgnores(cfg *Config) error {
	plan, err := prepareIgnorePlan(cfg)
	if err != nil {
		return err
	}
	return applyIgnorePlan(plan)
}

func applyIgnorePlan(plan *ignorePlan) error {
	if plan.git != nil {
		if err := plan.git.apply(); err != nil {
			return fmt.Errorf("update Git ignore rules: %w", err)
		}
	}
	if plan.svn != nil {
		if err := plan.svn.apply(); err != nil {
			return fmt.Errorf("update SVN ignore properties: %w", err)
		}
	}
	return nil
}

func prepareIgnorePlan(cfg *Config) (*ignorePlan, error) {
	cwd, err := getWorkingDirectory()
	if err != nil {
		return nil, err
	}
	plan := &ignorePlan{}
	if root, ok := findWorkingCopy(cwd, ".git"); ok {
		plan.git, err = prepareGitIgnore(root, cwd, cfg)
		if err != nil {
			return nil, err
		}
	}
	if root, ok := findWorkingCopy(cwd, ".svn"); ok {
		plan.svn, err = prepareSVNIgnore(root, cwd, cfg)
		if err != nil {
			return nil, err
		}
	}
	return plan, nil
}

func findWorkingCopy(start, marker string) (string, bool) {
	dir := filepath.Clean(start)
	for {
		if _, err := os.Stat(filepath.Join(dir, marker)); err == nil {
			return dir, true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return "", false
		}
		dir = parent
	}
}

func ignoredTargets(cfg *Config) []Dataset {
	result := make([]Dataset, 0, len(cfg.Datasets))
	for i := range cfg.Datasets {
		if cfg.Datasets[i].ShouldIgnore(cfg.Defaults.Ignore) {
			result = append(result, cfg.Datasets[i])
		}
	}
	return result
}

func relativeTarget(root, cwd string, ds Dataset) (string, error) {
	abs := ds.Target
	if !filepath.IsAbs(abs) {
		abs = filepath.Join(cwd, abs)
	}
	abs = filepath.Clean(abs)
	rel, err := filepath.Rel(root, abs)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) {
		return "", fmt.Errorf("dataset %q target %q is outside the version-control root %q", ds.ID, ds.Target, root)
	}
	if strings.ContainsAny(rel, "\r\n") {
		return "", fmt.Errorf("dataset %q target contains a newline and cannot be ignored safely", ds.ID)
	}
	return rel, nil
}

type gitIgnorePlan struct {
	path    string
	content []byte
	mode    os.FileMode
}

func prepareGitIgnore(root, cwd string, cfg *Config) (*gitIgnorePlan, error) {
	entries := make([]string, 0)
	for _, ds := range ignoredTargets(cfg) {
		rel, err := relativeTarget(root, cwd, ds)
		if err != nil {
			return nil, err
		}
		output, err := runVCSCommand("git", "-C", root, "ls-files", "--", filepath.ToSlash(rel))
		if err != nil {
			return nil, fmt.Errorf("check whether dataset %q is tracked by Git: %w", ds.ID, err)
		}
		if strings.TrimSpace(string(output)) != "" {
			return nil, fmt.Errorf("dataset %q target %q is already tracked by Git; untrack it before enabling ignore", ds.ID, ds.Target)
		}
		entries = append(entries, gitIgnorePattern(filepath.ToSlash(rel)))
	}
	sort.Strings(entries)
	entries = compactStrings(entries)

	path := filepath.Join(root, ".gitignore")
	content, err := os.ReadFile(path)
	mode := os.FileMode(0o644)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if info, statErr := os.Stat(path); statErr == nil {
		mode = info.Mode().Perm()
	}
	updated, err := replaceGitIgnoreBlock(content, entries)
	if err != nil {
		return nil, err
	}
	return &gitIgnorePlan{path: path, content: updated, mode: mode}, nil
}

func gitIgnorePattern(rel string) string {
	var pattern strings.Builder
	pattern.WriteByte('/')
	for _, char := range rel {
		if strings.ContainsRune(`\*?[]#! `, char) {
			pattern.WriteByte('\\')
		}
		pattern.WriteRune(char)
	}
	return pattern.String()
}

func replaceGitIgnoreBlock(content []byte, entries []string) ([]byte, error) {
	entries = compactStrings(append([]string(nil), entries...))
	newline := "\n"
	if bytes.Contains(content, []byte("\r\n")) {
		newline = "\r\n"
	}
	normalized := strings.ReplaceAll(string(content), "\r\n", "\n")
	lines := strings.Split(strings.TrimSuffix(normalized, "\n"), "\n")
	if len(lines) == 1 && lines[0] == "" {
		lines = nil
	}
	begin, end := -1, -1
	for i, line := range lines {
		switch line {
		case gitIgnoreBegin:
			if begin >= 0 {
				return nil, errors.New(".gitignore contains more than one Datum-managed block")
			}
			begin = i
		case gitIgnoreEnd:
			if end >= 0 {
				return nil, errors.New(".gitignore contains more than one Datum-managed block")
			}
			end = i
		}
	}
	if (begin >= 0) != (end >= 0) || begin > end {
		return nil, errors.New(".gitignore contains an incomplete Datum-managed block")
	}
	if begin >= 0 {
		lines = append(append([]string{}, lines[:begin]...), lines[end+1:]...)
	}
	for len(lines) > 0 && lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	}
	if len(entries) > 0 {
		if len(lines) > 0 {
			lines = append(lines, "")
		}
		lines = append(lines, gitIgnoreBegin)
		lines = append(lines, entries...)
		lines = append(lines, gitIgnoreEnd)
	}
	if len(lines) == 0 {
		return nil, nil
	}
	return []byte(strings.Join(lines, newline) + newline), nil
}

func (p *gitIgnorePlan) apply() error {
	existing, err := os.ReadFile(p.path)
	if errors.Is(err, os.ErrNotExist) && len(p.content) == 0 {
		return nil
	}
	if err == nil && bytes.Equal(existing, p.content) {
		return nil
	}
	return atomicWrite(p.path, p.content, p.mode)
}

type svnProperties struct {
	Targets []struct {
		Path       string `xml:"path,attr"`
		Properties []struct {
			Name  string `xml:"name,attr"`
			Value string `xml:",chardata"`
		} `xml:"property"`
	} `xml:"target"`
}

type svnPropertyUpdate struct {
	dir       string
	ignore    []string
	owned     []string
	hadIgnore bool
	hadOwned  bool
}

type svnIgnorePlan struct{ updates []svnPropertyUpdate }

type svnTempFile interface {
	WriteString(string) (int, error)
	Close() error
	Name() string
}

var createSVNTemp = func() (svnTempFile, error) {
	return os.CreateTemp("", "datum-svn-ignore-*")
}

func prepareSVNIgnore(root, cwd string, cfg *Config) (*svnIgnorePlan, error) {
	existingOwned, err := svnPropertyMap(root, svnOwnedProp, true)
	if err != nil {
		return nil, err
	}
	desired := map[string][]string{}
	for _, ds := range ignoredTargets(cfg) {
		rel, relErr := relativeTarget(root, cwd, ds)
		if relErr != nil {
			return nil, relErr
		}
		abs := filepath.Join(root, rel)
		parent := filepath.Dir(abs)
		basename := filepath.Base(abs)
		if strings.ContainsAny(basename, "*?[]\r\n") {
			return nil, fmt.Errorf("dataset %q target name %q cannot be represented as a precise SVN ignore pattern", ds.ID, basename)
		}
		if _, infoErr := runVCSCommand("svn", "info", parent); infoErr != nil {
			return nil, fmt.Errorf("dataset %q target parent %q is not a versioned SVN directory: %w", ds.ID, parent, infoErr)
		}
		if _, infoErr := runVCSCommand("svn", "info", abs); infoErr == nil {
			return nil, fmt.Errorf("dataset %q target %q is already tracked by SVN; untrack it before enabling ignore", ds.ID, ds.Target)
		}
		desired[parent] = append(desired[parent], basename)
	}

	dirs := map[string]bool{}
	for dir := range desired {
		dirs[dir] = true
	}
	for dir := range existingOwned {
		dirs[dir] = true
	}
	orderedDirs := make([]string, 0, len(dirs))
	for dir := range dirs {
		orderedDirs = append(orderedDirs, dir)
	}
	sort.Strings(orderedDirs)

	plan := &svnIgnorePlan{}
	for _, dir := range orderedDirs {
		current, hadIgnore, propErr := svnProperty(dir, "svn:ignore")
		if propErr != nil {
			return nil, propErr
		}
		owned := existingOwned[dir]
		user := subtractStrings(current, owned)
		newOwned := make([]string, 0)
		for _, entry := range compactStrings(desired[dir]) {
			if contains(owned, entry) || !contains(current, entry) {
				newOwned = append(newOwned, entry)
			}
		}
		newIgnore := compactStrings(append(user, newOwned...))
		plan.updates = append(plan.updates, svnPropertyUpdate{
			dir: dir, ignore: newIgnore, owned: compactStrings(newOwned),
			hadIgnore: hadIgnore, hadOwned: len(owned) > 0,
		})
	}
	return plan, nil
}

func svnPropertyMap(root, property string, recursive bool) (map[string][]string, error) {
	args := []string{"propget", property, "--xml"}
	if recursive {
		args = append(args, "--recursive")
	}
	args = append(args, root)
	output, err := runVCSCommand("svn", args...)
	if err != nil {
		// Subversion reports an absent property as an error. Confirm the working copy separately so
		// a missing client or invalid working copy is not mistaken for an empty property set.
		if _, infoErr := runVCSCommand("svn", "info", root); infoErr != nil {
			return nil, infoErr
		}
		return map[string][]string{}, nil
	}
	var parsed svnProperties
	if err := xml.Unmarshal(output, &parsed); err != nil {
		return nil, fmt.Errorf("parse SVN property output: %w", err)
	}
	result := map[string][]string{}
	for _, target := range parsed.Targets {
		for _, prop := range target.Properties {
			if prop.Name == property {
				path := target.Path
				if !filepath.IsAbs(path) {
					path = filepath.Join(root, path)
				}
				result[filepath.Clean(path)] = splitProperty(prop.Value)
			}
		}
	}
	return result, nil
}

func svnProperty(dir, property string) ([]string, bool, error) {
	properties, err := svnPropertyMap(dir, property, false)
	if err != nil {
		return nil, false, err
	}
	value, ok := properties[filepath.Clean(dir)]
	return value, ok, nil
}

func (p *svnIgnorePlan) apply() error {
	for _, update := range p.updates {
		if err := setSVNProperty(update.dir, "svn:ignore", update.ignore, update.hadIgnore); err != nil {
			return err
		}
		if err := setSVNProperty(update.dir, svnOwnedProp, update.owned, update.hadOwned); err != nil {
			return err
		}
	}
	return nil
}

func setSVNProperty(dir, property string, values []string, existed bool) error {
	if len(values) == 0 {
		if !existed {
			return nil
		}
		_, err := runVCSCommand("svn", "propdel", property, dir)
		return err
	}
	tmp, err := createSVNTemp()
	if err != nil {
		return err
	}
	name := tmp.Name()
	defer os.Remove(name)
	if _, err := tmp.WriteString(strings.Join(values, "\n") + "\n"); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	_, err = runVCSCommand("svn", "propset", property, "--file", name, dir)
	return err
}

func splitProperty(value string) []string {
	var result []string
	for _, line := range strings.Split(strings.ReplaceAll(value, "\r\n", "\n"), "\n") {
		if line != "" {
			result = append(result, line)
		}
	}
	return compactStrings(result)
}

func compactStrings(values []string) []string {
	sort.Strings(values)
	result := values[:0]
	for _, value := range values {
		if len(result) == 0 || result[len(result)-1] != value {
			result = append(result, value)
		}
	}
	return result
}

func subtractStrings(values, remove []string) []string {
	result := make([]string, 0, len(values))
	for _, value := range values {
		if !contains(remove, value) {
			result = append(result, value)
		}
	}
	return result
}

func contains(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}

func atomicWrite(path string, content []byte, mode os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, content, mode); err != nil {
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}
