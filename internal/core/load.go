package core

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/jprybylski/datum/internal/registry"
)

// expandScope decides, per dataset, whether ${NAME} references must resolve.
//
// A strict dataset is one the command is about to read sources for: every reference in it must
// name a set variable, as in datum 1.6. Every other dataset is expanded leniently: set
// variables are substituted, and a reference to an unset one stays exactly as written. So
// `fetch A` needs only A's variables, and audit, undelete and unlock need none (#42).
type expandScope struct {
	strict         func(id string) bool
	strictDefaults bool
}

var (
	scopeAllStrict  = expandScope{strict: func(string) bool { return true }, strictDefaults: true}
	scopeAllLenient = expandScope{strict: func(string) bool { return false }}
)

// scopeSelected is strict for the listed ids (all datasets when ids is empty) and lenient for
// the rest.
func scopeSelected(ids []string) expandScope {
	if len(ids) == 0 {
		return scopeAllStrict
	}
	which := make(map[string]bool, len(ids))
	for _, id := range ids {
		which[id] = true
	}
	return expandScope{strict: func(id string) bool { return which[id] }, strictDefaults: true}
}

// configInfo is what loadConfig knows about each dataset besides the decoded config, indexed
// like Config.Datasets.
type configInfo struct {
	// relTargets are the targets as written (cleaned), before --root made them absolute.
	relTargets []string
	// unresolved marks leniently expanded datasets whose target still holds a ${NAME}; datum
	// won't touch their files in this command, so target checks skip them.
	unresolved []bool
	// root is the absolute --root, or "" without one.
	root string
}

// readConfig loads a config the way datum 1.6 did: every reference must resolve and no
// managed-mode checks apply. Kept for tests and callers that need nothing else.
func readConfig(path string) (*Config, error) {
	cfg, _, err := loadConfig(path, "", Options{}, scopeAllStrict)
	return cfg, err
}

// loadConfig reads, expands, decodes and validates a config.
//
// In order:
//  1. --env-allow/--env-prefix are checked against every raw reference;
//  2. references are substituted per scope;
//  3. the document is decoded and defaulted, and each dataset gets the 1.6 source check;
//  4. --allow-types, --https-only and --git-https-only are checked on every dataset;
//  5. --root and --target-prefix are checked, and with --root, targets and relative file
//     sources are made absolute.
//
// Every failure names the dataset and field, never a substituted value.
func loadConfig(cfgPath, lockPath string, opts Options, scope expandScope) (*Config, *configInfo, error) {
	b, err := os.ReadFile(cfgPath)
	if err != nil {
		return nil, nil, err
	}

	// Parse into a YAML node first so environment values are substituted in scalar values, not
	// in the YAML source text. This matters for secrets containing YAML-significant characters
	// such as ':', '#', or newlines: they remain one value and cannot change the document's
	// structure.
	var document yaml.Node
	if err := yaml.Unmarshal(b, &document); err != nil {
		return nil, nil, err
	}

	resetSecrets()
	if len(opts.EnvAllow) > 0 || opts.EnvPrefix != "" {
		if err := checkEnvReferences(&document, opts); err != nil {
			return nil, nil, err
		}
	}
	if err := expandConfigNode(&document, scope); err != nil {
		return nil, nil, fmt.Errorf("expand environment variables: %w", err)
	}

	var c Config
	if err := document.Decode(&c); err != nil {
		return nil, nil, err
	}

	// Apply default values if not specified in the configuration
	if c.Defaults.Policy == "" {
		c.Defaults.Policy = "fail" // Default to strict mode
	}
	if c.Defaults.Algo == "" {
		c.Defaults.Algo = "sha256" // Default to SHA256 hashing
	}

	info := &configInfo{
		relTargets: make([]string, len(c.Datasets)),
		unresolved: make([]bool, len(c.Datasets)),
	}
	for i := range c.Datasets {
		ds := &c.Datasets[i]
		if err := validateDataset(ds); err != nil {
			return nil, nil, fmt.Errorf("dataset %d (%s): %w", i, ds.ID, err)
		}
		info.relTargets[i] = ds.Target
		info.unresolved[i] = !scope.strict(ds.ID) && strings.Contains(ds.Target, "${")
	}

	if err := checkSourcePolicy(&c, opts); err != nil {
		return nil, nil, err
	}
	if err := confineTargets(&c, info, cfgPath, lockPath, opts); err != nil {
		return nil, nil, err
	}
	return &c, info, nil
}

// expandConfigNode substitutes references in the document: each dataset strictly or leniently
// per scope, everything else per scope.strictDefaults.
func expandConfigNode(document *yaml.Node, scope expandScope) error {
	root := document
	if root.Kind == yaml.DocumentNode && len(root.Content) == 1 {
		root = root.Content[0]
	}
	if root.Kind != yaml.MappingNode {
		return expandEnvInYAML(document, !scope.strictDefaults)
	}
	for i := 0; i+1 < len(root.Content); i += 2 {
		key, value := root.Content[i], root.Content[i+1]
		if key.Value != "datasets" || value.Kind != yaml.SequenceNode {
			if err := expandEnvInYAML(value, !scope.strictDefaults); err != nil {
				return err
			}
			continue
		}
		for _, item := range value.Content {
			lenient := !scope.strict(rawDatasetID(item))
			if err := expandEnvInYAML(item, lenient); err != nil {
				return err
			}
		}
	}
	return nil
}

// rawDatasetID returns a dataset node's id as written. Ids are plain names (the schema pattern
// has no "$"), so the raw value is the id.
func rawDatasetID(item *yaml.Node) string {
	if item.Kind != yaml.MappingNode {
		return ""
	}
	for i := 0; i+1 < len(item.Content); i += 2 {
		if item.Content[i].Value == "id" {
			return item.Content[i+1].Value
		}
	}
	return ""
}

// checkEnvReferences fails on the first ${NAME} that --env-allow/--env-prefix doesn't allow,
// naming the dataset and field. It runs before any substitution, over the whole document.
func checkEnvReferences(document *yaml.Node, opts Options) error {
	allowed := make(map[string]bool, len(opts.EnvAllow))
	for _, name := range opts.EnvAllow {
		allowed[name] = true
	}
	ok := func(name string) bool {
		return allowed[name] || opts.EnvPrefix != "" && strings.HasPrefix(name, opts.EnvPrefix)
	}

	var walk func(node *yaml.Node, where string) error
	walk = func(node *yaml.Node, where string) error {
		switch node.Kind {
		case yaml.DocumentNode, yaml.SequenceNode:
			for i, child := range node.Content {
				next := where
				if node.Kind == yaml.SequenceNode {
					next = fmt.Sprintf("%s[%d]", where, i)
				}
				if err := walk(child, next); err != nil {
					return err
				}
			}
		case yaml.MappingNode:
			for i := 0; i+1 < len(node.Content); i += 2 {
				next := node.Content[i].Value
				if where != "" {
					next = where + "." + next
				}
				if err := walk(node.Content[i+1], next); err != nil {
					return err
				}
			}
		case yaml.ScalarNode:
			if node.Tag != "!!str" {
				return nil
			}
			for _, name := range envReferences(node.Value) {
				if !ok(name) {
					return fmt.Errorf("%s: environment variable %q is not allowed (see --env-allow/--env-prefix)", describeField(document, where), name)
				}
			}
		}
		return nil
	}
	return walk(document, "")
}

// describeField turns "datasets[2].source.url" into `dataset 2 (id): field "source.url"`.
func describeField(document *yaml.Node, where string) string {
	var index int
	if n, _ := fmt.Sscanf(where, "datasets[%d]", &index); n == 1 {
		field := where[strings.Index(where, "]")+1:]
		field = strings.TrimPrefix(field, ".")
		id := ""
		root := document
		if root.Kind == yaml.DocumentNode && len(root.Content) == 1 {
			root = root.Content[0]
		}
		for i := 0; i+1 < len(root.Content); i += 2 {
			if root.Content[i].Value == "datasets" && index < len(root.Content[i+1].Content) {
				id = rawDatasetID(root.Content[i+1].Content[index])
			}
		}
		return fmt.Sprintf("dataset %d (%s): field %q", index, id, field)
	}
	return fmt.Sprintf("field %q", where)
}

// envReferences lists the names of the ${NAME} references in value, skipping $$ escapes.
func envReferences(value string) []string {
	var names []string
	for i := 0; i+1 < len(value); {
		if value[i] != '$' {
			i++
			continue
		}
		switch value[i+1] {
		case '$':
			i += 2
		case '{':
			end := strings.IndexByte(value[i+2:], '}')
			if end < 0 {
				return names
			}
			names = append(names, value[i+2:i+2+end])
			i += end + 3
		default:
			i++
		}
	}
	return names
}

// checkSourcePolicy applies --allow-types, --https-only and --git-https-only to every source of
// every dataset, selected or not: a config that breaks them is refused as a whole.
func checkSourcePolicy(c *Config, opts Options) error {
	allowed := make(map[string]bool, len(opts.AllowTypes))
	for _, t := range opts.AllowTypes {
		allowed[t] = true
	}
	for i := range c.Datasets {
		ds := &c.Datasets[i]
		sources := ds.GetSources()
		for j, src := range sources {
			field := "source"
			if len(ds.Sources) > 0 {
				field = fmt.Sprintf("sources[%d]", j)
			}
			if len(allowed) > 0 && !allowed[src.Type] {
				return fmt.Errorf("dataset %d (%s): %s: source type %q is not allowed (allowed: %s)",
					i, ds.ID, field, src.Type, strings.Join(opts.AllowTypes, ", "))
			}
			urlChecked := opts.HTTPSOnly && (src.Type == "http" || src.Type == "git") ||
				opts.GitHTTPSOnly && src.Type == "git"
			if urlChecked && !strings.Contains(src.URL, "${") && !isHTTPS(src.URL) {
				flag := "--https-only"
				if !opts.HTTPSOnly {
					flag = "--git-https-only"
				}
				return fmt.Errorf("dataset %d (%s): %s.url: only https:// URLs are allowed (%s)", i, ds.ID, field, flag)
			}
		}
	}
	return nil
}

func isHTTPS(raw string) bool {
	return len(raw) > len("https://") && strings.EqualFold(raw[:len("https://")], "https://")
}

// confineTargets applies --root and --target-prefix, then (with --root) makes relative targets
// and relative file sources absolute under the root. Without either flag it changes nothing.
func confineTargets(c *Config, info *configInfo, cfgPath, lockPath string, opts Options) error {
	if opts.Root == "" && opts.TargetPrefix == "" {
		return nil
	}
	base := opts.Root
	if base == "" {
		base = "."
	}
	absRoot, err := filepath.Abs(base)
	if err != nil {
		return fmt.Errorf("--root: %w", err)
	}
	realRoot, err := filepath.EvalSymlinks(absRoot)
	if err != nil {
		return fmt.Errorf("--root: %w", err)
	}
	if opts.Root != "" {
		info.root = absRoot
	}

	var realPrefix string
	if opts.TargetPrefix != "" {
		prefix, err := cleanRelative(opts.TargetPrefix)
		if err != nil {
			return fmt.Errorf("--target-prefix: %w", err)
		}
		realPrefix, err = resolveUnder(realRoot, prefix)
		if err != nil {
			return fmt.Errorf("--target-prefix: %w", err)
		}
	}

	protected := map[string]string{}
	for _, p := range []struct{ name, path string }{{"config", cfgPath}, {"lockfile", lockPath}} {
		if p.path == "" {
			continue
		}
		if abs, err := filepath.Abs(p.path); err == nil {
			if real, err := resolveExisting(abs); err == nil {
				protected[real] = p.name
			}
		}
	}

	for i := range c.Datasets {
		ds := &c.Datasets[i]
		if !info.unresolved[i] {
			if err := checkTarget(ds.Target, realRoot, realPrefix, protected); err != nil {
				return fmt.Errorf("dataset %d (%s): target: %w", i, ds.ID, err)
			}
			info.relTargets[i] = filepath.Clean(filepath.FromSlash(ds.Target))
		}
		if opts.Root == "" {
			continue
		}
		if !filepath.IsAbs(ds.Target) {
			ds.Target = filepath.Join(absRoot, filepath.FromSlash(ds.Target))
		}
		absolutizeFileSource(&ds.Source, absRoot)
		for j := range ds.Sources {
			absolutizeFileSource(&ds.Sources[j], absRoot)
		}
	}
	return nil
}

func absolutizeFileSource(src *registry.Source, root string) {
	if src.Type == "file" && src.Path != "" && !filepath.IsAbs(src.Path) && !strings.Contains(src.Path, "${") {
		src.Path = filepath.Join(root, filepath.FromSlash(src.Path))
	}
}

// checkTarget enforces one target's confinement: relative, no "..", inside the root after
// following symlinks, under the prefix if one is set, and not the config or lockfile itself.
func checkTarget(target, realRoot, realPrefix string, protected map[string]string) error {
	rel, err := cleanRelative(target)
	if err != nil {
		return err
	}
	real, err := resolveUnder(realRoot, rel)
	if err != nil {
		return err
	}
	if real == realRoot {
		return errors.New("must not be the root folder itself")
	}
	if realPrefix != "" && !within(realPrefix, real) {
		return errors.New("must lie under --target-prefix")
	}
	if name, ok := protected[real]; ok {
		return fmt.Errorf("must not be the %s", name)
	}
	return nil
}

// cleanRelative rejects empty, absolute and ".."-containing paths and returns the cleaned path.
func cleanRelative(p string) (string, error) {
	if p == "" {
		return "", errors.New("must not be empty")
	}
	slashed := strings.ReplaceAll(p, `\`, "/")
	if strings.HasPrefix(slashed, "/") || filepath.IsAbs(p) || filepath.VolumeName(p) != "" {
		return "", errors.New("must be a relative path")
	}
	for _, part := range strings.Split(slashed, "/") {
		if part == ".." {
			return "", errors.New(`must not contain ".."`)
		}
	}
	return filepath.Clean(filepath.FromSlash(slashed)), nil
}

// resolveUnder joins rel to realRoot, follows symlinks through the part of the path that exists,
// and fails if the result leaves realRoot.
func resolveUnder(realRoot, rel string) (string, error) {
	real, err := resolveExisting(filepath.Join(realRoot, rel))
	if err != nil {
		return "", err
	}
	if !within(realRoot, real) {
		return "", errors.New("resolves outside --root (through a symbolic link)")
	}
	return real, nil
}

// resolveExisting follows symlinks in the longest existing prefix of an absolute path and
// appends the rest unchanged.
func resolveExisting(p string) (string, error) {
	p = filepath.Clean(p)
	var rest []string
	for {
		real, err := filepath.EvalSymlinks(p)
		if err == nil {
			for i := len(rest) - 1; i >= 0; i-- {
				real = filepath.Join(real, rest[i])
			}
			return real, nil
		}
		if !errors.Is(err, os.ErrNotExist) {
			return "", err
		}
		parent := filepath.Dir(p)
		if parent == p {
			return "", err
		}
		rest = append(rest, filepath.Base(p))
		p = parent
	}
}

// within reports whether p is base or inside it.
func within(base, p string) bool {
	rel, err := filepath.Rel(base, p)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator)) && !filepath.IsAbs(rel)
}
