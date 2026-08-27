//go:build !windows

package proc

import "syscall"

// newSysProcAttr: nothing to hide on Unix platforms.
func newSysProcAttr() *syscall.SysProcAttr { return &syscall.SysProcAttr{} }

// applyHidden: no-op outside Windows.
func applyHidden(*syscall.SysProcAttr) {}
