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
	"os"

	"example.com/datum/internal/core"
	// Side-effect imports: These imports don't use any exported symbols,
	// but they run init() functions that register handlers with the registry.
	// The underscore (_) tells Go we're importing for side effects only.
	//
	// Go learning note: init() functions in these packages run automatically
	// before main(), registering their handlers in the global registry.
	_ "example.com/datum/internal/handlers/command"
	_ "example.com/datum/internal/handlers/file"
	_ "example.com/datum/internal/handlers/http"
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

// main is the program entry point.
//
// Execution flow:
//  1. Parse command-line flags (--config, --lock)
//  2. Get the subcommand (check or fetch)
//  3. Dispatch to the appropriate core function
//  4. Exit with the returned status code
//
// Exit codes:
//
//	0 = Success
//	1 = Verification failed or fetch error
//	2 = Configuration error or invalid usage
func main() {
	// Define command-line flags
	// StringVar binds a flag to a variable. Format: (varPtr, flagName, defaultValue, description)
	var cfgPath, lockPath string
	flag.StringVar(&cfgPath, "config", ".data.yaml", "path to config YAML")
	flag.StringVar(&lockPath, "lock", ".data.lock.yaml", "path to lock YAML")

	// Parse flags from os.Args[1:]
	// After this call, flag.Args() contains non-flag arguments (the subcommand and its args)
	flag.Parse()

	// Require at least one non-flag argument (the subcommand)
	if flag.NArg() < 1 {
		usage()
		os.Exit(2) // Exit code 2 = invalid usage
	}

	// Get the subcommand (first non-flag argument)
	cmd := flag.Arg(0)

	// Dispatch to the appropriate handler based on subcommand
	switch cmd {
	case "check":
		// Verify all datasets against the lockfile
		code := core.Check(cfgPath, lockPath)
		os.Exit(code)

	case "fetch":
		// Fetch specific datasets (or all if none specified)
		// flag.Args() returns all non-flag arguments, [1:] skips the subcommand itself
		ids := flag.Args()[1:]
		code := core.Fetch(cfgPath, lockPath, ids)
		os.Exit(code)

	default:
		// Unknown subcommand - show usage and exit
		usage()
		os.Exit(2)
	}
}
