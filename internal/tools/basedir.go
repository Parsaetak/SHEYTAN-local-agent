package tools

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

var (
	baseMu  sync.RWMutex
	baseDir string
)

var (
	// ErrPathOutsideBase is returned when an agent path would escape the
	// canonical application/tool directory.
	ErrPathOutsideBase = errors.New("tools: path escapes base directory")

	// ErrBaseDirUnset is returned when a safe path cannot be resolved because
	// the canonical base directory has not been configured.
	ErrBaseDirUnset = errors.New("tools: base directory is not configured")
)

// SetBaseDir sets the canonical base directory that relative paths in ALL
// tools resolve against. Call once during runtime stack construction.
func SetBaseDir(dir string) {
	dir = strings.TrimSpace(dir)

	if dir != "" {
		if abs, err := filepath.Abs(dir); err == nil {
			dir = filepath.Clean(abs)
		}
	}

	baseMu.Lock()
	baseDir = dir
	baseMu.Unlock()
}

// BaseDir returns the current canonical base directory.
func BaseDir() string {
	baseMu.RLock()
	defer baseMu.RUnlock()

	return baseDir
}

// ResolvePath resolves p against the canonical base directory.
//
// This compatibility helper preserves the historical string-only API.
// Invalid paths resolve to an empty string. New security-sensitive code
// should use ResolvePathChecked so the caller receives the exact error.
func ResolvePath(p string) string {
	resolved, err := ResolvePathChecked(p)
	if err != nil {
		return ""
	}

	return resolved
}

// ResolvePathChecked resolves p and guarantees that the resulting path stays
// inside BaseDir().
//
// Absolute paths are rejected unless they are already inside BaseDir().
// Relative paths are resolved beneath BaseDir(). Home-directory expansion
// is deliberately not supported because "~" can otherwise escape the tool
// jail.
//
// This function is the security boundary for the generic filesystem-facing
// tools.
func ResolvePathChecked(p string) (string, error) {
	p = strings.TrimSpace(p)

	if p == "" {
		return "", errors.New("tools: path is empty")
	}

	base := BaseDir()
	if base == "" {
		return "", ErrBaseDirUnset
	}

	baseAbs, err := filepath.Abs(base)
	if err != nil {
		return "", err
	}

	baseAbs = filepath.Clean(baseAbs)

	// Normalize Windows separators before validation so ../ and ..\ are
	// treated identically.
	normalized := strings.ReplaceAll(p, "\\", string(filepath.Separator))

	// Reject tilde expansion rather than silently reaching outside the base.
	if normalized == "~" ||
		strings.HasPrefix(normalized, "~"+string(filepath.Separator)) {
		return "", ErrPathOutsideBase
	}

	var candidate string

	if filepath.IsAbs(normalized) {
		candidate = normalized
	} else {
		candidate = filepath.Join(baseAbs, normalized)
	}

	candidateAbs, err := filepath.Abs(candidate)
	if err != nil {
		return "", err
	}

	candidateAbs = filepath.Clean(candidateAbs)

	if !pathWithinBase(candidateAbs, baseAbs) {
		return "", ErrPathOutsideBase
	}

	return candidateAbs, nil
}

// ResolvePathWithinBase resolves a caller-supplied path beneath an explicit
// base directory. This is useful for isolated components such as Coding Lab
// workspaces where BaseDir() is intentionally not the relevant root.
func ResolvePathWithinBase(base, p string) (string, error) {
	base = strings.TrimSpace(base)
	p = strings.TrimSpace(p)

	if base == "" {
		return "", ErrBaseDirUnset
	}

	if p == "" {
		return "", errors.New("tools: path is empty")
	}

	baseAbs, err := filepath.Abs(base)
	if err != nil {
		return "", err
	}

	baseAbs = filepath.Clean(baseAbs)

	normalized := strings.ReplaceAll(p, "\\", string(filepath.Separator))

	if normalized == "~" ||
		strings.HasPrefix(normalized, "~"+string(filepath.Separator)) {
		return "", ErrPathOutsideBase
	}

	var candidate string

	if filepath.IsAbs(normalized) {
		candidate = normalized
	} else {
		candidate = filepath.Join(baseAbs, normalized)
	}

	candidateAbs, err := filepath.Abs(candidate)
	if err != nil {
		return "", err
	}

	candidateAbs = filepath.Clean(candidateAbs)

	if !pathWithinBase(candidateAbs, baseAbs) {
		return "", ErrPathOutsideBase
	}

	return candidateAbs, nil
}

// pathWithinBase returns true when path is base itself or a descendant of it.
func pathWithinBase(path, base string) bool {
	path = filepath.Clean(path)
	base = filepath.Clean(base)

	if path == base {
		return true
	}

	rel, err := filepath.Rel(base, path)
	if err != nil {
		return false
	}

	if rel == "." {
		return true
	}

	return rel != ".." &&
		!strings.HasPrefix(
			rel,
			".."+string(filepath.Separator),
		)
}

// SafeJoin is a convenience wrapper for code that needs to construct a path
// from several user-controlled relative components while retaining the same
// jail guarantee.
func SafeJoin(parts ...string) (string, error) {
	if len(parts) == 0 {
		return "", errors.New("tools: no path supplied")
	}

	base := BaseDir()
	if base == "" {
		return "", ErrBaseDirUnset
	}

	joined := filepath.Join(parts...)
	return ResolvePathWithinBase(base, joined)
}

// SafeExistingPath resolves a path and verifies that the final filesystem
// object still remains inside the canonical base.
//
// Symlink targets are checked as well when the target exists, preventing a
// seemingly safe path such as "logs/current" from escaping through a symlink.
func SafeExistingPath(p string) (string, error) {
	resolved, err := ResolvePathChecked(p)
	if err != nil {
		return "", err
	}

	info, statErr := os.Lstat(resolved)
	if statErr != nil {
		if errors.Is(statErr, os.ErrNotExist) {
			// The path itself is still safe even when it does not exist yet.
			return resolved, nil
		}

		return "", statErr
	}

	if info.Mode()&os.ModeSymlink != 0 {
		target, err := filepath.EvalSymlinks(resolved)
		if err != nil {
			return "", err
		}

		if !pathWithinBase(target, BaseDir()) {
			return "", ErrPathOutsideBase
		}
	}

	return resolved, nil
}

// UnsafeAbsolutePath reports whether p is an absolute filesystem path outside
// the canonical tool base. It is intentionally small so existing tools can
// use it when deciding whether an explicitly supplied destination is safe.
func UnsafeAbsolutePath(p string) bool {
	p = strings.TrimSpace(p)

	if p == "" || !filepath.IsAbs(p) {
		return false
	}

	base := BaseDir()
	if base == "" {
		return true
	}

	resolved, err := ResolvePathChecked(p)
	return err != nil || resolved == ""
}
