// Package main is the SHEYTAN-Local-Agent entry point.
// On Windows with no args, launches the native desktop GUI (Fyne).
// On non-Windows, or with a CLI subcommand, dispatches to the CLI.
package main

import (
	"os"

	"github.com/sheytan/local-agent/cmd"
)

func main() {
	os.Exit(cmd.RunWithDefaultFn(runDefault))
}

// runDefault is overridden by the per-OS file (e.g. main_windows.go launches
// the native GUI; main_other.go prints a fallback message).
var runDefault = func() int { return 0 }
