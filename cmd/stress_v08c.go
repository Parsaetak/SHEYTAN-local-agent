package cmd

// helpers for stress_v08b (kept separate to avoid import noise in the main test file)
import (
	"os"
	"os/exec"
)

func osMkdirTemp(pattern string) (string, error) { return os.MkdirTemp("", pattern) }
func osRemoveAll(path string) error              { return os.RemoveAll(path) }

func runGitInit(dir string) error {
	return exec.Command("git", "init", dir).Run()
}

func runGit(dir, args string) (string, error) {
	cmd := exec.Command("git", "log")
	cmd.Dir = dir
	out, err := cmd.CombinedOutput()
	return string(out), err
}
