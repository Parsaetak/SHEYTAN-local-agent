//go:build !windows

package main

import (
	"fmt"
	"os"

	"github.com/sheytan/local-agent/internal/config"
)

func init() {
	// On non-Windows (Linux/macOS dev box), the native Fyne GUI is not
	// available — print a hint and exit.
	runDefault = func() int {
		fmt.Fprintf(os.Stderr,
			"%s v%s — native desktop GUI is only available on Windows.\n"+
				"On this %s/%s box you can still use the CLI subcommands:\n"+
				"  serve, install, doctor, sysinfo, stress, version, help.\n",
			config.AppName, config.AppVersion, "linux", "amd64")
		return 1
	}
}
