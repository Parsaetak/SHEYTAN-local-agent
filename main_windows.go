//go:build windows

package main

import (
	"github.com/sheytan/local-agent/internal/ui"
)

func init() {
	// On Windows with no subcommand, launch the native desktop GUI.
	runDefault = func() int {
		ui.RunDesktop()
		return 0
	}
}
