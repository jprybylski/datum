package main

import (
    "flag"
    "fmt"
    "os"

    "example.com/pinup/internal/core"

    // Side-effect imports register handlers via init()
    _ "example.com/pinup/internal/handlers/file"
    _ "example.com/pinup/internal/handlers/http"
    _ "example.com/pinup/internal/handlers/command"
)

func usage() {
    fmt.Println(`pinup - verify/fetch external data by config+lock

Usage:
  pinup [--config .data.yaml] [--lock .data.lock.yaml] check
  pinup [--config .data.yaml] [--lock .data.lock.yaml] fetch [ID ...]
`)
}

func main() {
    var cfgPath, lockPath string
    flag.StringVar(&cfgPath, "config", ".data.yaml", "path to config YAML")
    flag.StringVar(&lockPath, "lock", ".data.lock.yaml", "path to lock YAML")
    flag.Parse()
    if flag.NArg() < 1 {
        usage()
        os.Exit(2)
    }

    cmd := flag.Arg(0)
    switch cmd {
    case "check":
        code := core.Check(cfgPath, lockPath)
        os.Exit(code)
    case "fetch":
        ids := flag.Args()[1:]
        code := core.Fetch(cfgPath, lockPath, ids)
        os.Exit(code)
    default:
        usage()
        os.Exit(2)
    }
}
