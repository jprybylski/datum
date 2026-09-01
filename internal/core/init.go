package core

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/jprybylski/datum/internal/registry"
)

var datasetIDPattern = regexp.MustCompile(`^[a-zA-Z0-9_-]+$`)

var (
	marshalConfig = yaml.Marshal
	writeConfig   = atomicWrite
	prepareIgnore = prepareIgnorePlan
	applyIgnore   = applyIgnorePlan
)

// InitOptions contains values accepted by datum init. The Set fields distinguish omitted flags
// from explicit empty/false values when deciding which terminal prompts to show.
type InitOptions struct {
	ID, Type, Source, Target, Desc, Policy string
	Ignore, Empty                          bool
	DescSet, PolicySet, IgnoreSet          bool
}

// Init creates a new configuration and can optionally add one basic HTTP or file dataset.
func Init(configPath string, options InitOptions, in io.Reader, out io.Writer, interactive bool) int {
	if _, err := os.Stat(configPath); err == nil {
		fmt.Fprintf(out, "init error: config already exists: %s\n", configPath)
		return 2
	} else if !errors.Is(err, os.ErrNotExist) {
		fmt.Fprintf(out, "init error: inspect config path: %v\n", err)
		return 2
	}
	if options.Empty {
		if options.ID != "" || options.Type != "" || options.Source != "" || options.Target != "" || options.DescSet {
			fmt.Fprintln(out, "init error: --empty cannot be combined with dataset-specific flags")
			return 2
		}
		return writeEmptyConfig(configPath, options, out)
	}

	reader := bufio.NewReader(in)
	if interactive {
		if !options.PolicySet {
			options.Policy = prompt(reader, out, "Default policy (fail/update/log)", "fail")
		}
		if !options.IgnoreSet {
			answer := prompt(reader, out, "Ignore fetched targets in a detected VCS? (y/N)", "n")
			options.Ignore = yes(answer)
		}
		wantsDataset := options.ID != "" || options.Type != "" || options.Source != "" || options.Target != "" || options.DescSet
		if !wantsDataset {
			wantsDataset = yes(prompt(reader, out, "Add an initial dataset? (y/N)", "n"))
		}
		if !wantsDataset {
			return writeEmptyConfig(configPath, options, out)
		}
		options.ID = promptMissing(reader, out, "Dataset ID", options.ID, "")
		options.Type = promptMissing(reader, out, "Source type (http/file)", options.Type, "")
		options.Source = promptMissing(reader, out, "Source URL or path", options.Source, "")
		options.Target = promptMissing(reader, out, "Target path", options.Target, "")
		if !options.DescSet {
			options.Desc = prompt(reader, out, "Description", options.ID)
		}
	}
	if options.Desc == "" {
		options.Desc = options.ID
	}
	if options.Policy == "" {
		options.Policy = "fail"
	}
	if err := validateInitOptions(options); err != nil {
		fmt.Fprintf(out, "init error: %v\n", err)
		return 2
	}

	source := registry.Source{Type: options.Type}
	if options.Type == "http" {
		source.URL = options.Source
	} else {
		source.Path = options.Source
	}
	cfg := &Config{
		Version:  1,
		Defaults: Defaults{Policy: options.Policy, Algo: "sha256", Ignore: options.Ignore},
		Datasets: []Dataset{{
			ID: options.ID, Desc: options.Desc, Source: source, Target: options.Target,
		}},
	}
	ignorePlan, err := prepareIgnore(cfg)
	if err != nil {
		fmt.Fprintf(out, "init error: prepare ignore rules: %v\n", err)
		return 2
	}
	data, err := marshalConfig(cfg)
	if err != nil {
		fmt.Fprintf(out, "init error: encode config: %v\n", err)
		return 1
	}
	if err := writeConfig(configPath, data, 0o644); err != nil {
		fmt.Fprintf(out, "init error: write config: %v\n", err)
		return 1
	}
	if err := applyIgnore(ignorePlan); err != nil {
		if removeErr := os.Remove(configPath); removeErr != nil {
			fmt.Fprintf(out, "init error: apply ignore rules: %v (also could not remove config: %v)\n", err, removeErr)
		} else {
			fmt.Fprintf(out, "init error: apply ignore rules: %v\n", err)
		}
		return 1
	}
	fmt.Fprintf(out, "Initialized %s with dataset %q.\n", configPath, options.ID)
	return 0
}

func writeEmptyConfig(configPath string, options InitOptions, out io.Writer) int {
	if options.Policy == "" {
		options.Policy = "fail"
	}
	if options.Policy != "fail" && options.Policy != "update" && options.Policy != "log" {
		fmt.Fprintln(out, "init error: --policy must be fail, update, or log")
		return 2
	}
	type emptyConfig struct {
		Version  int       `yaml:"version"`
		Defaults *Defaults `yaml:"defaults,omitempty"`
		Datasets []Dataset `yaml:"datasets"`
	}
	document := emptyConfig{Version: 1, Datasets: []Dataset{}}
	if options.PolicySet || options.IgnoreSet || options.Policy != "fail" || options.Ignore {
		document.Defaults = &Defaults{Policy: options.Policy, Algo: "sha256", Ignore: options.Ignore}
	}
	data, err := marshalConfig(document)
	if err != nil {
		fmt.Fprintf(out, "init error: encode config: %v\n", err)
		return 1
	}
	if err := writeConfig(configPath, data, 0o644); err != nil {
		fmt.Fprintf(out, "init error: write config: %v\n", err)
		return 1
	}
	fmt.Fprintf(out, "Initialized %s with no datasets.\n", configPath)
	return 0
}

func yes(value string) bool {
	return strings.EqualFold(value, "y") || strings.EqualFold(value, "yes")
}

func validateInitOptions(options InitOptions) error {
	missing := make([]string, 0, 4)
	for _, field := range []struct{ name, value string }{
		{"--id", options.ID}, {"--type", options.Type},
		{"--source", options.Source}, {"--target", options.Target},
	} {
		if field.value == "" {
			missing = append(missing, field.name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("missing required init flags: %s", strings.Join(missing, ", "))
	}
	if !datasetIDPattern.MatchString(options.ID) {
		return errors.New("--id must contain only letters, numbers, underscores, or hyphens")
	}
	if options.Type != "http" && options.Type != "file" {
		return fmt.Errorf("--type must be %q or %q", "http", "file")
	}
	if options.Policy != "fail" && options.Policy != "update" && options.Policy != "log" {
		return errors.New("--policy must be fail, update, or log")
	}
	return nil
}

func promptMissing(reader *bufio.Reader, out io.Writer, label, value, defaultValue string) string {
	if value != "" {
		return value
	}
	return prompt(reader, out, label, defaultValue)
}

func prompt(reader *bufio.Reader, out io.Writer, label, defaultValue string) string {
	if defaultValue == "" {
		fmt.Fprintf(out, "%s: ", label)
	} else {
		fmt.Fprintf(out, "%s [%s]: ", label, defaultValue)
	}
	value, _ := reader.ReadString('\n')
	value = strings.TrimSpace(value)
	if value == "" {
		return defaultValue
	}
	return value
}
