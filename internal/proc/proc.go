// Package proc launches subprocesses without ever flashing a console
// window on Windows.
//
// v1.0.4 fix — the "extra terminal that is always open": SHEYTAN is a GUI
// app (-H=windowsgui), but every child process it spawned before
// (llama-server.exe, cmd.exe for the shell tool, wmic/powershell for
// hardware probes, git, python, node, tar) is a CONSOLE application.
// Windows gives each console app launched from a GUI process its own
// brand-new console window — the engine alone kept one terminal open for
// the whole session, and every hardware probe flashed another.
//
// Every process the app spawns now goes through proc.Command /
// proc.CommandContext (or proc.Hide), which set
// CREATE_NO_WINDOW | HideWindow on Windows and are no-ops elsewhere.
package proc

import (
	"context"
	"os/exec"
)

// Hide marks the command so Windows never creates a console window for
// it. It is safe to call on every platform (no-op outside Windows).
func Hide(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = newSysProcAttr()
		return
	}
	applyHidden(cmd.SysProcAttr)
}

// Command builds an exec.Cmd that never opens a console window on Windows.
// Use it instead of exec.Command everywhere the app spawns a subprocess.
func Command(name string, args ...string) *exec.Cmd {
	cmd := exec.Command(name, args...)
	Hide(cmd)
	return cmd
}

// CommandContext is the context-aware variant of Command.
func CommandContext(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	Hide(cmd)
	return cmd
}
