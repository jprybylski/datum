//go:build git

package main

import _ "github.com/jprybylski/datum/internal/handlers/git"

func init() { features = append(features, "git") }
