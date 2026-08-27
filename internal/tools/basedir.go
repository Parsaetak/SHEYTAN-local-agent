// Package tools — base directory resolution for cross-tool interoperability.
//
// The LLM frequently chains tools: it writes "sales.csv" with `files`, then
// profiles it with `dataAnalysis`, maybe processes it with `shell`, and
// commits it with `git`. If each tool interpreted relative paths against a
// different working directory (the OS cwd can be anything on Windows when
// launched from Explorer), those chains would break.
//
// SetBaseDir pins ONE canonical base (the portable app folder) for every
// tool, so "sales.csv" means the same file everywhere.
package tools

import (
	"os"
	"path/filepath"
	"sync"
)

var (
	baseMu  sync.RWMutex
	baseDir string
)

// SetBaseDir sets the canonical base directory that relative paths in ALL
// tools resolve against. Call once at stack construction (runtime.Stack).
func SetBaseDir(dir string) {
	baseMu.Lock()
	defer baseMu.Unlock()
	baseDir = dir
}

// BaseDir returns the current canonical base directory ("" if unset).
func BaseDir() string {
	baseMu.RLock()
	defer baseMu.RUnlock()
	return baseDir
}

// ResolvePath resolves `p` against the canonical base directory:
//   - absolute paths pass through untouched
//   - relative paths (including "./x" and "~/x") resolve under BaseDir()
//     so every tool agrees on where agent files live
func ResolvePath(p string) string {
	if p == "" {
		return p
	}
	// Expand ~ to the user's home first (friendly for LLM output).
	if len(p) >= 2 && p[0] == '~' && (p[1] == '/' || p[1] == '\\') {
		if home, err := os.UserHomeDir(); err == nil {
			p = filepath.Join(home, p[2:])
		}
	}
	if filepath.IsAbs(p) {
		return filepath.Clean(p)
	}
	baseMu.RLock()
	base := baseDir
	baseMu.RUnlock()
	if base == "" {
		return filepath.Clean(p)
	}
	return filepath.Join(base, p)
}
