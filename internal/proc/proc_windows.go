//go:build windows

package proc

import "syscall"

// CREATE_NO_WINDOW: the child runs detached from any console and Windows
// never allocates a new one. Combined with HideWindow this covers both
// console and GUI subsystem children.
const createNoWindow = 0x08000000

// newSysProcAttr returns attributes for a hidden, console-less child.
func newSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow,
	}
}

// applyHidden mutates existing attributes in place.
func applyHidden(attr *syscall.SysProcAttr) {
	attr.HideWindow = true
	attr.CreationFlags |= createNoWindow
}
