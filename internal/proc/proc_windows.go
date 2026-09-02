//go:build windows

package proc

import (
	"fmt"
	"os/exec"
	"strconv"
	"syscall"
)

// CREATE_NO_WINDOW: the child runs detached from any console and Windows
// never allocates a new one.
const createNoWindow = 0x08000000

// CREATE_NEW_PROCESS_GROUP gives the command its own process group so the
// timeout handler can terminate the complete descendant tree.
const createNewProcessGroup = 0x00000200

// newSysProcAttr returns attributes for a hidden, console-less child with its
// own process group.
func newSysProcAttr() *syscall.SysProcAttr {
	return &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow | createNewProcessGroup,
	}
}

// applyHidden mutates existing attributes in place.
func applyHidden(attr *syscall.SysProcAttr) {
	if attr == nil {
		return
	}

	attr.HideWindow = true
	attr.CreationFlags |= createNoWindow | createNewProcessGroup
}

// killProcessTree asks Windows to terminate the complete descendant tree.
//
// taskkill /T is used because terminating only the shell process leaves
// grandchildren such as compilers, test runners, or interpreters alive.
func killProcessTree(cmd *exec.Cmd) error {
	if cmd == nil || cmd.Process == nil {
		return nil
	}

	pid := strconv.Itoa(cmd.Process.Pid)

	kill := exec.Command(
		"taskkill",
		"/PID",
		pid,
		"/T",
		"/F",
	)

	if err := kill.Run(); err != nil {
		return fmt.Errorf(
			"proc: terminate process tree %s: %w",
			pid,
			err,
		)
	}

	return nil
}
