// Datum is a data pinning tool that tracks external data sources with cryptographic fingerprints.
//
// This is the main entry point for the datum CLI application. It parses command-line
// arguments and dispatches to the appropriate handler function in the core package.
//
// Go beginners: The main package and main() function are special in Go - they define
// the entry point for executable programs. Libraries use other package names.
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"golang.org/x/term"

	"github.com/jprybylski/datum/internal/core"
	// Side-effect imports: These imports don't use any exported symbols,
	// but they run init() functions that register handlers with the registry.
	// The underscore (_) tells Go we're importing for side effects only.
	//
	// Go learning note: init() functions in these packages run automatically
	// before main(), registering their handlers in the global registry.
	_ "github.com/jprybylski/datum/internal/handlers/command"
	_ "github.com/jprybylski/datum/internal/handlers/file"
	_ "github.com/jprybylski/datum/internal/handlers/http"
)

// version is set at build time via -ldflags "-X main.version=vX.Y.Z" (see .goreleaser.yml).
// Locally built binaries (plain "go build") keep the "dev" default.
var version = "dev"

// features lists optional capabilities compiled into this build, such as "git" (see
// handlers_git.go). It's reported by `datum --json --version`.
var features = []string{}

// usage prints help text to stdout.
//
// This is called when the user provides no arguments or an invalid command.
// The help text uses Go's raw string literals (backticks) which preserve
// formatting and don't require escaping newlines.
func usage() {
	fmt.Print(`datum - verify/fetch external data by config+lock

Usage:
  datum [--config .data.yaml] [--lock .data.lock.yaml] [--timeout 5m] [--concurrency 1] [--no-color] [--json] check
  datum [--config .data.yaml] [--lock .data.lock.yaml] [--timeout 5m] [--concurrency 1] [--no-color] [--json] fetch [ID ...]
  datum [--config .data.yaml] [--lock .data.lock.yaml] [--yes] delete ID [ID ...]
  datum [--lock .data.lock.yaml] undelete ID [ID ...]
  datum [--config .data.yaml] [--lock .data.lock.yaml] [--yes] unlock ID [ID ...]
  datum [--config .data.yaml] [--lock .data.lock.yaml] [--no-color] [--json] audit
  datum [--no-color] [--json] types [TYPE ...]
  datum schema
  datum [--config .data.yaml] init [--empty [--policy fail|update|log] [--ignore] | --id ID --type http|file --source VALUE --target PATH [--desc TEXT] [--policy fail|update|log] [--ignore]]
  datum [--json] --version

Flags for running datum on configs you didn't write (all optional, all before the command;
see docs/embedding.md):
  --no-vcs                  never run git/svn or edit ignore rules (or DATUM_NO_VCS=1)
  --allow-types LIST        refuse any other source type, e.g. http,file,git (or DATUM_ALLOW_TYPES)
  --root DIR                resolve targets and relative file sources in DIR, and keep targets inside it
  --target-prefix PATH      every target must lie under PATH (relative to --root)
  --env-allow LIST          only these ${NAME} variables may be referenced
  --env-prefix PREFIX       only ${NAME} variables starting with PREFIX may be referenced
  --https-only              refuse http and git sources that aren't https://
  --git-https-only          refuse git sources that aren't https://; never use SSH keys or agents
  --cache-dir DIR           cache folder for git sources (instead of XDG_CACHE_HOME)
  --report-only             check: write nothing; "update" datasets are reported, not fetched
  --stage-dir DIR           fetch: write targets under DIR instead of in place (needs --lock-out)
  --lock-out PATH           fetch: write the new lockfile to PATH (needs --stage-dir)
  --verify-local            audit: hash local targets and report drift from the lockfile
`)
}

// run parses args and dispatches to the appropriate core function, returning the process exit
// code. It's factored out of main() so it can be exercised directly in tests without needing to
// spawn a subprocess or deal with flag.Parse's default os.Exit-on-error behavior.
//
// Exit codes:
//
//	0 = Success
//	1 = Verification failed or fetch error
//	2 = Configuration error or invalid usage
func run(args []string) int {
	// Use a dedicated FlagSet (rather than the global flag.CommandLine) so parse errors return
	// to the caller instead of calling os.Exit directly - that's what makes this testable.
	fs := flag.NewFlagSet("datum", flag.ContinueOnError)
	fs.SetOutput(io.Discard)

	var cfgPath, lockPath string
	var timeout time.Duration
	var concurrency int
	var showVersion bool
	var noColor bool
	var jsonOutput bool
	var yes bool
	fs.StringVar(&cfgPath, "config", ".data.yaml", "path to config YAML")
	fs.StringVar(&lockPath, "lock", ".data.lock.yaml", "path to lock YAML")
	fs.DurationVar(&timeout, "timeout", 5*time.Minute, "overall timeout for the whole check/fetch run (e.g. 30s, 5m, 1h); 0 disables it")
	fs.IntVar(&concurrency, "concurrency", 1, "number of datasets to process in parallel (default: sequential)")
	fs.BoolVar(&showVersion, "version", false, "print the datum version and exit")
	fs.BoolVar(&noColor, "no-color", false, "disable colored output (also honored via the NO_COLOR env var)")
	fs.BoolVar(&jsonOutput, "json", false, "print results as a single JSON document instead of colorized text")
	fs.BoolVar(&yes, "yes", false, "skip delete/unlock's confirmation prompt (for scripts/CI; has no effect on other commands)")

	// Opt-in flags for running datum on configs it didn't write (see core.Options). Two of them
	// also have environment variables; a flag on the command line wins.
	opts := core.OptionsFromEnv()
	var allowTypes, envAllow string
	fs.BoolVar(&opts.NoVCS, "no-vcs", opts.NoVCS, "never run git/svn or edit ignore rules")
	fs.StringVar(&allowTypes, "allow-types", "", "comma-separated source types a config may use")
	fs.StringVar(&opts.Root, "root", "", "folder targets and relative file sources resolve in, and targets must stay inside")
	fs.StringVar(&opts.TargetPrefix, "target-prefix", "", "folder (relative to --root) every target must lie under")
	fs.StringVar(&envAllow, "env-allow", "", "comma-separated ${NAME} variables a config may reference")
	fs.StringVar(&opts.EnvPrefix, "env-prefix", "", "prefix every referenced ${NAME} variable must have")
	fs.BoolVar(&opts.HTTPSOnly, "https-only", false, "refuse http and git sources that aren't https://")
	fs.BoolVar(&opts.GitHTTPSOnly, "git-https-only", false, "refuse git sources that aren't https://")
	fs.StringVar(&opts.CacheDir, "cache-dir", "", "cache folder for git sources")
	fs.BoolVar(&opts.ReportOnly, "report-only", false, "check: write nothing")
	fs.StringVar(&opts.StageDir, "stage-dir", "", "fetch: write targets under this folder")
	fs.StringVar(&opts.LockOut, "lock-out", "", "fetch: write the new lockfile here")
	fs.BoolVar(&opts.VerifyLocal, "verify-local", false, "audit: report local drift")

	if err := fs.Parse(args); err != nil {
		usage()
		return 2
	}

	if noColor {
		core.NoColor = true
	}
	if jsonOutput {
		core.JSONOutput = true
	}

	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "allow-types":
			opts.AllowTypes = core.SplitList(allowTypes)
		case "env-allow":
			opts.EnvAllow = core.SplitList(envAllow)
		}
	})

	if showVersion {
		if jsonOutput {
			return printVersionJSON()
		}
		fmt.Println("datum", version)
		return 0
	}

	// Require at least one non-flag argument (the subcommand)
	if fs.NArg() < 1 {
		usage()
		return 2
	}

	if concurrency < 1 {
		concurrency = 1
	}
	opts.Concurrency = concurrency

	ctx := context.Background()
	if timeout > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, timeout)
		defer cancel()
	}

	// Get the subcommand (first non-flag argument)
	cmd := fs.Arg(0)

	// Dispatch to the appropriate handler based on subcommand
	switch cmd {
	case "check":
		// Verify all datasets against the lockfile
		return core.CheckWith(ctx, cfgPath, lockPath, opts)

	case "fetch":
		// Fetch specific datasets (or all if none specified)
		// fs.Args()[1:] skips the subcommand itself
		ids := fs.Args()[1:]
		return core.FetchWith(ctx, cfgPath, lockPath, ids, opts)

	case "delete":
		// Remove tracked local files for the given IDs and mark them deleted in the lockfile
		ids := fs.Args()[1:]
		return core.DeleteWith(cfgPath, lockPath, ids, yes, os.Stdin, os.Stdout, opts)

	case "undelete":
		// Clear the deleted flag for the given IDs so check/fetch resume tracking them
		ids := fs.Args()[1:]
		return core.Undelete(lockPath, ids, os.Stdout)

	case "unlock":
		// Permanently forget lockfile state for the given IDs (config or orphaned)
		ids := fs.Args()[1:]
		return core.Unlock(cfgPath, lockPath, ids, yes, os.Stdin, os.Stdout)

	case "audit":
		// Report every dataset's config+lockfile state (ok/pending/deleted/orphaned)
		return core.AuditWith(cfgPath, lockPath, opts)

	case "types":
		// List available source types, or show complete fields for selected types.
		return core.Types(fs.Args()[1:])

	case "schema":
		// Print the exact configuration schema shipped with this build.
		return core.Schema(os.Stdout)

	case "init":
		return runInit(cfgPath, fs.Args()[1:], opts.NoVCS)

	default:
		// Unknown subcommand - show usage and exit
		usage()
		return 2
	}
}

func runInit(configPath string, args []string, noVCS bool) int {
	fs := flag.NewFlagSet("datum init", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	var options core.InitOptions
	fs.StringVar(&options.ID, "id", "", "dataset identifier")
	fs.StringVar(&options.Type, "type", "", "source type: http or file")
	fs.StringVar(&options.Source, "source", "", "source URL or file path")
	fs.StringVar(&options.Target, "target", "", "local target path")
	fs.StringVar(&options.Desc, "desc", "", "dataset description")
	fs.StringVar(&options.Policy, "policy", "", "default policy: fail, update, or log")
	fs.BoolVar(&options.Ignore, "ignore", false, "ignore fetched targets in a detected VCS")
	fs.BoolVar(&options.Empty, "empty", false, "create a valid configuration with no datasets")
	if err := fs.Parse(args); err != nil || fs.NArg() != 0 {
		usage()
		return 2
	}
	fs.Visit(func(f *flag.Flag) {
		switch f.Name {
		case "desc":
			options.DescSet = true
		case "policy":
			options.PolicySet = true
		case "ignore":
			options.IgnoreSet = true
		}
	})
	options.NoVCS = noVCS
	interactive := term.IsTerminal(int(os.Stdin.Fd())) && !core.JSONOutput
	return core.Init(configPath, options, os.Stdin, os.Stdout, interactive)
}

// printVersionJSON prints `datum --json --version` (#39): the version without its "v", the
// optional features in this build, and the report format.
func printVersionJSON() int {
	data, err := json.MarshalIndent(struct {
		Format   string   `json:"format"`
		Version  string   `json:"version"`
		Features []string `json:"features"`
	}{Format: core.ReportFormat, Version: strings.TrimPrefix(version, "v"), Features: features}, "", "  ")
	if err != nil {
		fmt.Printf("json encode error: %v\n", err)
		return 1
	}
	fmt.Println(string(data))
	return 0
}

// main is the program entry point.
func main() {
	os.Exit(run(os.Args[1:]))
}
