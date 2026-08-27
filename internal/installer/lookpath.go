package installer

import (
	"os/exec"
)

// lookPathImpl wraps exec.LookPath so we can mock it in tests if needed.
func lookPathImpl(name string) (string, error) {
	return exec.LookPath(name)
}
