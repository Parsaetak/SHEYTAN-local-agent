// Package lab contains the autonomous coding-laboratory runtime.
//
// The workspace layer is deliberately independent from the LLM and agent
// packages: the model should operate on an isolated task workspace rather
// than directly on the user's source tree.
package lab

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

var (
	ErrInvalidSource    = errors.New("lab: invalid source repository")
	ErrInvalidWorkspace = errors.New("lab: invalid workspace path")
)

// Workspace describes one isolated coding task.
type Workspace struct {
	ID        string    `json:"id"`
	Source    string    `json:"source"`
	Path      string    `json:"path"`
	CreatedAt time.Time `json:"createdAt"`
}

// WorkspaceManager owns task workspaces under a single laboratory root.
type WorkspaceManager struct {
	Root string
}

// NewWorkspaceManager creates a manager rooted at root. The root is made
// absolute and created immediately so later task creation is deterministic.
func NewWorkspaceManager(root string) (*WorkspaceManager, error) {
	if strings.TrimSpace(root) == "" {
		return nil, fmt.Errorf("lab: workspace root is empty")
	}

	abs, err := filepath.Abs(root)
	if err != nil {
		return nil, fmt.Errorf("lab: resolve workspace root: %w", err)
	}

	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, fmt.Errorf("lab: create workspace root: %w", err)
	}

	return &WorkspaceManager{Root: abs}, nil
}

// Create copies a source repository into a fresh isolated workspace.
// The source .git directory is excluded because the task workspace is meant
// to be disposable; Git state is handled separately by the patch/verification
// layer.
func (m *WorkspaceManager) Create(ctx context.Context, source string) (*Workspace, error) {
	if m == nil || m.Root == "" {
		return nil, ErrInvalidWorkspace
	}
	if ctx == nil {
		ctx = context.Background()
	}

	sourceAbs, err := filepath.Abs(source)
	if err != nil {
		return nil, fmt.Errorf("lab: resolve source: %w", err)
	}

	info, err := os.Stat(sourceAbs)
	if err != nil {
		return nil, fmt.Errorf("lab: stat source: %w", err)
	}
	if !info.IsDir() {
		return nil, ErrInvalidSource
	}

	rootAbs, err := filepath.Abs(m.Root)
	if err != nil {
		return nil, fmt.Errorf("lab: resolve root: %w", err)
	}

	// Never create a task workspace inside the source repository, and never
	// use a workspace root that is inside the source repository.
	if sameOrWithin(sourceAbs, rootAbs) || sameOrWithin(rootAbs, sourceAbs) {
		return nil, fmt.Errorf("%w: source and workspace root overlap", ErrInvalidWorkspace)
	}

	id, err := newWorkspaceID()
	if err != nil {
		return nil, fmt.Errorf("lab: generate workspace id: %w", err)
	}

	dst := filepath.Join(rootAbs, id)
	if err := os.MkdirAll(dst, 0o755); err != nil {
		return nil, fmt.Errorf("lab: create task workspace: %w", err)
	}

	if err := copyTree(ctx, sourceAbs, dst); err != nil {
		_ = os.RemoveAll(dst)
		return nil, err
	}

	return &Workspace{
		ID:        id,
		Source:    sourceAbs,
		Path:      dst,
		CreatedAt: time.Now().UTC(),
	}, nil
}

// Remove deletes one workspace. The path must remain underneath the manager's
// root, preventing a cleanup request from deleting an arbitrary filesystem
// location.
func (m *WorkspaceManager) Remove(workspace *Workspace) error {
	if m == nil || workspace == nil {
		return ErrInvalidWorkspace
	}

	pathAbs, err := filepath.Abs(workspace.Path)
	if err != nil {
		return fmt.Errorf("lab: resolve workspace: %w", err)
	}

	rootAbs, err := filepath.Abs(m.Root)
	if err != nil {
		return fmt.Errorf("lab: resolve workspace root: %w", err)
	}

	if !sameOrWithin(pathAbs, rootAbs) || pathAbs == rootAbs {
		return ErrInvalidWorkspace
	}

	if err := os.RemoveAll(pathAbs); err != nil {
		return fmt.Errorf("lab: remove workspace %q: %w", pathAbs, err)
	}

	return nil
}

// PathFor returns a path inside the workspace after validating that it cannot
// escape through absolute paths or .. components.
func (w *Workspace) PathFor(relative string) (string, error) {
	if w == nil || w.Path == "" {
		return "", ErrInvalidWorkspace
	}

	if filepath.IsAbs(relative) {
		return "", ErrInvalidWorkspace
	}

	base, err := filepath.Abs(w.Path)
	if err != nil {
		return "", fmt.Errorf("lab: resolve workspace: %w", err)
	}

	candidate, err := filepath.Abs(filepath.Join(base, relative))
	if err != nil {
		return "", fmt.Errorf("lab: resolve workspace path: %w", err)
	}

	if !sameOrWithin(candidate, base) && candidate != base {
		return "", ErrInvalidWorkspace
	}

	return candidate, nil
}

func copyTree(ctx context.Context, src, dst string) error {
	return filepath.WalkDir(src, func(path string, entry fs.DirEntry, walkErr error) error {
		if err := ctx.Err(); err != nil {
			return err
		}

		if walkErr != nil {
			return fmt.Errorf("lab: inspect %q: %w", path, walkErr)
		}

		rel, err := filepath.Rel(src, path)
		if err != nil {
			return fmt.Errorf("lab: relative path for %q: %w", path, err)
		}

		if rel == "." {
			return nil
		}

		// Git metadata belongs to the source repository, not the disposable
		// task workspace. This also avoids copying potentially large objects.
		if entry.IsDir() && filepath.Base(path) == ".git" {
			return fs.SkipDir
		}

		target := filepath.Join(dst, rel)

		if entry.IsDir() {
			if err := os.MkdirAll(target, 0o755); err != nil {
				return fmt.Errorf("lab: create directory %q: %w", target, err)
			}
			return nil
		}

		info, err := entry.Info()
		if err != nil {
			return fmt.Errorf("lab: inspect file %q: %w", path, err)
		}

		// External symlinks are intentionally omitted so a disposable
		// workspace cannot transparently reference files outside its boundary.
		if info.Mode()&os.ModeSymlink != 0 {
			return nil
		}

		if !info.Mode().IsRegular() {
			return nil
		}

		if err := copyFile(path, target, info.Mode().Perm()); err != nil {
			return err
		}

		return nil
	})
}

func copyFile(src, dst string, perm os.FileMode) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf("lab: open %q: %w", src, err)
	}
	defer in.Close()

	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return fmt.Errorf("lab: create parent for %q: %w", dst, err)
	}

	out, err := os.OpenFile(dst, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, perm)
	if err != nil {
		return fmt.Errorf("lab: create %q: %w", dst, err)
	}

	_, copyErr := io.Copy(out, in)
	closeErr := out.Close()

	if copyErr != nil {
		_ = os.Remove(dst)
		return fmt.Errorf("lab: copy %q: %w", src, copyErr)
	}

	if closeErr != nil {
		return fmt.Errorf("lab: close %q: %w", dst, closeErr)
	}

	return nil
}

func newWorkspaceID() (string, error) {
	var b [8]byte

	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}

	return fmt.Sprintf(
		"task-%s-%s",
		time.Now().UTC().Format("20060102-150405"),
		hex.EncodeToString(b[:]),
	), nil
}

func sameOrWithin(path, root string) bool {
	path = filepath.Clean(path)
	root = filepath.Clean(root)

	if path == root {
		return true
	}

	rel, err := filepath.Rel(root, path)
	if err != nil {
		return false
	}

	return rel != ".." &&
		!strings.HasPrefix(rel, ".."+string(filepath.Separator)) &&
		rel != "" &&
		rel != "."
}
