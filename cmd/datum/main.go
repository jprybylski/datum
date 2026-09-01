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
	"flag"
	"fmt"
	"io"
	"os"
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
  datum [--config .data.yaml] init [--id ID] [--type http|file] [--source VALUE] [--target PATH] [--desc TEXT] [--policy fail|update|log] [--ignore]
  datum --version
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

	if showVersion {
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
		return core.Check(ctx, cfgPath, lockPath, concurrency)

	case "fetch":
		// Fetch specific datasets (or all if none specified)
		// fs.Args()[1:] skips the subcommand itself
		ids := fs.Args()[1:]
		return core.Fetch(ctx, cfgPath, lockPath, ids, concurrency)

	case "delete":
		// Remove tracked local files for the given IDs and mark them deleted in the lockfile
		ids := fs.Args()[1:]
		return core.Delete(cfgPath, lockPath, ids, yes, os.Stdin, os.Stdout)

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
		return core.Audit(cfgPath, lockPath)

	case "types":
		// List available source types, or show complete fields for selected types.
		return core.Types(fs.Args()[1:])

	case "schema":
		// Print the exact configuration schema shipped with this build.
		return core.Schema(os.Stdout)

	case "init":
		return runInit(cfgPath, fs.Args()[1:])

	default:
		// Unknown subcommand - show usage and exit
		usage()
		return 2
	}
}

func runInit(configPath string, args []string) int {
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
	interactive := term.IsTerminal(int(os.Stdin.Fd()))
	return core.Init(configPath, options, os.Stdin, os.Stdout, interactive)
}

// main is the program entry point.
func main() {
	os.Exit(run(os.Args[1:]))
}
