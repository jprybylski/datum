// Datum is a data pinning tool that tracks external data sources with cryptographic fingerprints.
//
// This is the main entry point for the datum CLI application. It parses command-line
// arguments and dispatches to the appropriate handler function in the core package.
//
// Go beginners: The main package and main() function are special in Go - they define
// the entry point for executable programs. Libraries use other package names.
package main

import (
	"flag"
	"fmt"
	"io"
	"os"

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

// usage prints help text to stdout.
//
// This is called when the user provides no arguments or an invalid command.
// The help text uses Go's raw string literals (backticks) which preserve
// formatting and don't require escaping newlines.
func usage() {
	fmt.Print(`datum - verify/fetch external data by config+lock

Usage:
  datum [--config .data.yaml] [--lock .data.lock.yaml] check
  datum [--config .data.yaml] [--lock .data.lock.yaml] fetch [ID ...]
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
	fs.StringVar(&cfgPath, "config", ".data.yaml", "path to config YAML")
	fs.StringVar(&lockPath, "lock", ".data.lock.yaml", "path to lock YAML")

	if err := fs.Parse(args); err != nil {
		usage()
		return 2
	}

	// Require at least one non-flag argument (the subcommand)
	if fs.NArg() < 1 {
		usage()
		return 2
	}

	// Get the subcommand (first non-flag argument)
	cmd := fs.Arg(0)

	// Dispatch to the appropriate handler based on subcommand
	switch cmd {
	case "check":
		// Verify all datasets against the lockfile
		return core.Check(cfgPath, lockPath)

	case "fetch":
		// Fetch specific datasets (or all if none specified)
		// fs.Args()[1:] skips the subcommand itself
		ids := fs.Args()[1:]
		return core.Fetch(cfgPath, lockPath, ids)

	default:
		// Unknown subcommand - show usage and exit
		usage()
		return 2
	}
}

// main is the program entry point.
func main() {
	os.Exit(run(os.Args[1:]))
}
