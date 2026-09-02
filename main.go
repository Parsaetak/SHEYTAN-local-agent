// Package main is the SHEYTAN-Local-Agent entry point.
//
// With no command, the per-platform entrypoint launches the native Wails
// desktop application. Explicit CLI subcommands continue through cmd.
package main

import (
	"os"

	"github.com/Parsaetak/SHEYTAN-local-agent/cmd"
)

func main() {
	os.Exit(cmd.RunWithDefaultFn(runDefault))
}

// runDefault is overridden by the per-platform entrypoint.
// Both Windows and Linux launch the native Wails desktop application.
var runDefault = func() int { return 0 }
