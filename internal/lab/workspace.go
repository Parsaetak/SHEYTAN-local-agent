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
	ErrPromotionFailed  = errors.New("lab: promotion failed")
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
		return nil, fmt.Errorf(
			"lab: resolve workspace root: %w",
			err,
		)
	}

	abs = filepath.Clean(abs)

	if err := os.MkdirAll(abs, 0o755); err != nil {
		return nil, fmt.Errorf(
			"lab: create workspace root: %w",
			err,
		)
	}

	return &WorkspaceManager{
		Root: abs,
	}, nil
}

// Create copies a source repository into a fresh isolated workspace.
//
// The source .git directory is excluded because the task workspace is meant
// to be disposable. Git state remains in the original source repository and
// is preserved separately during promotion/snapshot operations.
func (m *WorkspaceManager) Create(
	ctx context.Context,
	source string,
) (*Workspace, error) {
	if m == nil || m.Root == "" {
		return nil, ErrInvalidWorkspace
	}

	if ctx == nil {
		ctx = context.Background()
	}

	sourceAbs, err := filepath.Abs(source)
	if err != nil {
		return nil, fmt.Errorf(
			"lab: resolve source: %w",
			err,
		)
	}

	sourceAbs = filepath.Clean(sourceAbs)

	info, err := os.Stat(sourceAbs)
	if err != nil {
		return nil, fmt.Errorf(
			"lab: stat source: %w",
			err,
		)
	}

	if !info.IsDir() {
		return nil, ErrInvalidSource
	}

	rootAbs, err := filepath.Abs(m.Root)
	if err != nil {
		return nil, fmt.Errorf(
			"lab: resolve root: %w",
			err,
		)
	}

	rootAbs = filepath.Clean(rootAbs)

	// Never create a task workspace inside the source repository, and never
	// use a workspace root that is inside the source repository.
	if sameOrWithin(sourceAbs, rootAbs) ||
		sameOrWithin(rootAbs, sourceAbs) {
		return nil, fmt.Errorf(
			"%w: source and workspace root overlap",
			ErrInvalidWorkspace,
		)
	}

	id, err := newWorkspaceID()
	if err != nil {
		return nil, fmt.Errorf(
			"lab: generate workspace id: %w",
			err,
		)
	}

	dst := filepath.Join(
		rootAbs,
		id,
	)

	if err := os.MkdirAll(dst, 0o755); err != nil {
		return nil, fmt.Errorf(
			"lab: create task workspace: %w",
			err,
		)
	}

	if err := copyTree(
		ctx,
		sourceAbs,
		dst,
	); err != nil {
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

// Snapshot creates a complete pre-promotion copy of the original source.
//
// The snapshot is created BEFORE Promote() mutates the source. Source Git
// metadata is included so the snapshot remains a faithful recovery copy.
func (m *WorkspaceManager) Snapshot(
	ctx context.Context,
	workspace *Workspace,
) (string, error) {
	if err := m.validateWorkspace(workspace); err != nil {
		return "", err
	}

	if ctx == nil {
		ctx = context.Background()
	}

	source := workspace.Source

	if strings.TrimSpace(source) == "" {
		return "", ErrInvalidSource
	}

	info, err := os.Stat(source)
	if err != nil {
		return "", fmt.Errorf(
			"lab: stat source before snapshot: %w",
			err,
		)
	}

	if !info.IsDir() {
		return "", ErrInvalidSource
	}

	snapshotID, err := newSnapshotID()
	if err != nil {
		return "", fmt.Errorf(
			"lab: generate snapshot id: %w",
			err,
		)
	}

	snapshotRoot := filepath.Join(
		m.Root,
		"snapshots",
		snapshotID,
	)

	if sameOrWithin(snapshotRoot, source) ||
		sameOrWithin(source, snapshotRoot) {
		return "", fmt.Errorf(
			"%w: snapshot overlaps source",
			ErrInvalidWorkspace,
		)
	}

	if err := os.MkdirAll(snapshotRoot, 0o755); err != nil {
		return "", fmt.Errorf(
			"lab: create snapshot directory: %w",
			err,
		)
	}

	if err := copyTreePreserveGit(
		ctx,
		source,
		snapshotRoot,
	); err != nil {
		_ = os.RemoveAll(snapshotRoot)

		return "", fmt.Errorf(
			"lab: snapshot source: %w",
			err,
		)
	}

	return snapshotRoot, nil
}

// Promote snapshots the original source first, then mirrors the Lab workspace
// into the source.
//
// The source .git directory is deliberately excluded from all destructive
// synchronization. The Lab workspace itself is never removed here.
//
// If snapshot creation fails, the source is untouched.
func (m *WorkspaceManager) Promote(
	ctx context.Context,
	workspace *Workspace,
) (string, error) {
	if err := m.validateWorkspace(workspace); err != nil {
		return "", err
	}

	if ctx == nil {
		ctx = context.Background()
	}

	if err := ensureDirectory(workspace.Path); err != nil {
		return "", fmt.Errorf(
			"%w: invalid workspace: %v",
			ErrPromotionFailed,
			err,
		)
	}

	if err := ensureDirectory(workspace.Source); err != nil {
		return "", fmt.Errorf(
			"%w: invalid source: %v",
			ErrPromotionFailed,
			err,
		)
	}

	// CRITICAL: snapshot must succeed before the first source mutation.
	snapshotPath, err := m.Snapshot(
		ctx,
		workspace,
	)
	if err != nil {
		return "", errors.Join(
			ErrPromotionFailed,
			err,
		)
	}

	if err := mirrorWorkspaceToSource(
		ctx,
		workspace.Path,
		workspace.Source,
	); err != nil {
		return snapshotPath, errors.Join(
			ErrPromotionFailed,
			err,
		)
	}

	return snapshotPath, nil
}

// Remove deletes one workspace. The path must remain underneath the manager's
// root, preventing a cleanup request from deleting an arbitrary filesystem
// location.
func (m *WorkspaceManager) Remove(
	workspace *Workspace,
) error {
	if m == nil || workspace == nil {
		return ErrInvalidWorkspace
	}

	pathAbs, err := filepath.Abs(workspace.Path)
	if err != nil {
		return fmt.Errorf(
			"lab: resolve workspace: %w",
			err,
		)
	}

	pathAbs = filepath.Clean(pathAbs)

	rootAbs, err := filepath.Abs(m.Root)
	if err != nil {
		return fmt.Errorf(
			"lab: resolve workspace root: %w",
			err,
		)
	}

	rootAbs = filepath.Clean(rootAbs)

	if !sameOrWithin(pathAbs, rootAbs) ||
		pathAbs == rootAbs {
		return ErrInvalidWorkspace
	}

	if err := os.RemoveAll(pathAbs); err != nil {
		return fmt.Errorf(
			"lab: remove workspace %q: %w",
			pathAbs,
			err,
		)
	}

	return nil
}

// PathFor returns a path inside the workspace after validating that it cannot
// escape through absolute paths or .. components.
func (w *Workspace) PathFor(relative string) (string, error) {
	if w == nil || w.Path == "" {
		return "", ErrInvalidWorkspace
	}

	relative = strings.TrimSpace(relative)

	if relative == "" {
		return filepath.Abs(w.Path)
	}

	normalized := strings.ReplaceAll(
		relative,
		"\\",
		string(filepath.Separator),
	)

	if filepath.IsAbs(normalized) {
		return "", ErrInvalidWorkspace
	}

	base, err := filepath.Abs(w.Path)
	if err != nil {
		return "", fmt.Errorf(
			"lab: resolve workspace: %w",
			err,
		)
	}

	base = filepath.Clean(base)

	candidate, err := filepath.Abs(
		filepath.Join(
			base,
			normalized,
		),
	)
	if err != nil {
		return "", fmt.Errorf(
			"lab: resolve workspace path: %w",
			err,
		)
	}

	candidate = filepath.Clean(candidate)

	if !sameOrWithin(candidate, base) &&
		candidate != base {
		return "", ErrInvalidWorkspace
	}

	// Resolve the nearest existing ancestor so an intermediate symlink cannot
	// redirect a future write outside the workspace.
	existing, err := resolveExistingPrefix(candidate)
	if err != nil {
		return "", err
	}

	if !sameOrWithin(existing, base) &&
		existing != base {
		return "", ErrInvalidWorkspace
	}

	return candidate, nil
}

func (m *WorkspaceManager) validateWorkspace(
	workspace *Workspace,
) error {
	if m == nil || strings.TrimSpace(m.Root) == "" {
		return ErrInvalidWorkspace
	}

	if workspace == nil ||
		strings.TrimSpace(workspace.Path) == "" ||
		strings.TrimSpace(workspace.Source) == "" {
		return ErrInvalidWorkspace
	}

	rootAbs, err := filepath.Abs(m.Root)
	if err != nil {
		return ErrInvalidWorkspace
	}

	pathAbs, err := filepath.Abs(workspace.Path)
	if err != nil {
		return ErrInvalidWorkspace
	}

	sourceAbs, err := filepath.Abs(workspace.Source)
	if err != nil {
		return ErrInvalidSource
	}

	rootAbs = filepath.Clean(rootAbs)
	pathAbs = filepath.Clean(pathAbs)
	sourceAbs = filepath.Clean(sourceAbs)

	if !sameOrWithin(pathAbs, rootAbs) ||
		pathAbs == rootAbs {
		return ErrInvalidWorkspace
	}

	if sameOrWithin(sourceAbs, rootAbs) ||
		sameOrWithin(rootAbs, sourceAbs) {
		return fmt.Errorf(
			"%w: source and workspace root overlap",
			ErrInvalidWorkspace,
		)
	}

	return nil
}

func mirrorWorkspaceToSource(
	ctx context.Context,
	workspaceRoot string,
	sourceRoot string,
) error {
	workspaceRoot, err := filepath.Abs(workspaceRoot)
	if err != nil {
		return err
	}

	sourceRoot, err = filepath.Abs(sourceRoot)
	if err != nil {
		return err
	}

	workspaceRoot = filepath.Clean(workspaceRoot)
	sourceRoot = filepath.Clean(sourceRoot)

	if sameOrWithin(sourceRoot, workspaceRoot) ||
		sameOrWithin(workspaceRoot, sourceRoot) {
		return fmt.Errorf(
			"%w: source and workspace overlap",
			ErrInvalidWorkspace,
		)
	}

	// First remove source entries that are not present in the workspace.
	// .git is always preserved.
	if err := removeSourceEntriesNotInWorkspace(
		ctx,
		workspaceRoot,
		sourceRoot,
	); err != nil {
		return err
	}

	// Then copy workspace contents into source.
	return copyTreeContents(
		ctx,
		workspaceRoot,
		sourceRoot,
	)
}

func removeSourceEntriesNotInWorkspace(
	ctx context.Context,
	workspaceRoot string,
	sourceRoot string,
) error {
	return filepath.WalkDir(
		sourceRoot,
		func(
			path string,
			entry fs.DirEntry,
			walkErr error,
		) error {
			if err := ctx.Err(); err != nil {
				return err
			}

			if walkErr != nil {
				return fmt.Errorf(
					"lab: inspect source %q: %w",
					path,
					walkErr,
				)
			}

			if path == sourceRoot {
				return nil
			}

			rel, err := filepath.Rel(
				sourceRoot,
				path,
			)
			if err != nil {
				return err
			}

			if rel == ".git" ||
				strings.HasPrefix(
					rel,
					".git"+string(filepath.Separator),
				) {
				if entry.IsDir() {
					return fs.SkipDir
				}

				return nil
			}

			target := filepath.Join(
				workspaceRoot,
				rel,
			)

			_, statErr := os.Lstat(target)

			if statErr == nil {
				if entry.IsDir() {
					return nil
				}

				return nil
			}

			if !errors.Is(
				statErr,
				os.ErrNotExist,
			) {
				return statErr
			}

			if entry.IsDir() {
				if err := os.RemoveAll(path); err != nil {
					return fmt.Errorf(
						"lab: remove stale directory %q: %w",
						path,
						err,
					)
				}

				return fs.SkipDir
			}

			if err := os.Remove(path); err != nil {
				return fmt.Errorf(
					"lab: remove stale source entry %q: %w",
					path,
					err,
				)
			}

			return nil
		},
	)
}

func copyTree(
	ctx context.Context,
	src string,
	dst string,
) error {
	return filepath.WalkDir(
		src,
		func(
			path string,
			entry fs.DirEntry,
			walkErr error,
		) error {
			if err := ctx.Err(); err != nil {
				return err
			}

			if walkErr != nil {
				return fmt.Errorf(
					"lab: inspect %q: %w",
					path,
					walkErr,
				)
			}

			rel, err := filepath.Rel(
				src,
				path,
			)
			if err != nil {
				return fmt.Errorf(
					"lab: relative path for %q: %w",
					path,
					err,
				)
			}

			if rel == "." {
				return nil
			}

			// Disposable workspace never receives Git metadata.
			if rel == ".git" ||
				strings.HasPrefix(
					rel,
					".git"+string(filepath.Separator),
				) {
				if entry.IsDir() {
					return fs.SkipDir
				}

				return nil
			}

			target := filepath.Join(
				dst,
				rel,
			)

			if entry.IsDir() {
				if err := os.MkdirAll(
					target,
					0o755,
				); err != nil {
					return fmt.Errorf(
						"lab: create directory %q: %w",
						target,
						err,
					)
				}

				return nil
			}

			info, err := entry.Info()
			if err != nil {
				return fmt.Errorf(
					"lab: inspect file %q: %w",
					path,
					err,
				)
			}

			// External symlinks are intentionally omitted.
			if info.Mode()&os.ModeSymlink != 0 {
				return nil
			}

			if !info.Mode().IsRegular() {
				return nil
			}

			return copyFile(
				path,
				target,
				info.Mode().Perm(),
			)
		},
	)
}

func copyTreePreserveGit(
	ctx context.Context,
	src string,
	dst string,
) error {
	return filepath.WalkDir(
		src,
		func(
			path string,
			entry fs.DirEntry,
			walkErr error,
		) error {
			if err := ctx.Err(); err != nil {
				return err
			}

			if walkErr != nil {
				return fmt.Errorf(
					"lab: inspect %q: %w",
					path,
					walkErr,
				)
			}

			rel, err := filepath.Rel(
				src,
				path,
			)
			if err != nil {
				return fmt.Errorf(
					"lab: relative path for %q: %w",
					path,
					err,
				)
			}

			if rel == "." {
				return nil
			}

			target := filepath.Join(
				dst,
				rel,
			)

			if entry.IsDir() {
				if err := os.MkdirAll(
					target,
					0o755,
				); err != nil {
					return fmt.Errorf(
						"lab: create snapshot directory %q: %w",
						target,
						err,
					)
				}

				return nil
			}

			info, err := entry.Info()
			if err != nil {
				return fmt.Errorf(
					"lab: inspect snapshot entry %q: %w",
					path,
					err,
				)
			}

			// Never follow symlinks while making a recovery snapshot.
			if info.Mode()&os.ModeSymlink != 0 {
				return nil
			}

			if !info.Mode().IsRegular() {
				return nil
			}

			return copyFile(
				path,
				target,
				info.Mode().Perm(),
			)
		},
	)
}

func copyTreeContents(
	ctx context.Context,
	src string,
	dst string,
) error {
	return filepath.WalkDir(
		src,
		func(
			path string,
			entry fs.DirEntry,
			walkErr error,
		) error {
			if err := ctx.Err(); err != nil {
				return err
			}

			if walkErr != nil {
				return fmt.Errorf(
					"lab: inspect workspace %q: %w",
					path,
					walkErr,
				)
			}

			rel, err := filepath.Rel(
				src,
				path,
			)
			if err != nil {
				return err
			}

			if rel == "." {
				return nil
			}

			// The disposable workspace does not contain .git in normal
			// operation, but keep the guard permanent so promotion can never
			// replace source Git metadata.
			if rel == ".git" ||
				strings.HasPrefix(
					rel,
					".git"+string(filepath.Separator),
				) {
				if entry.IsDir() {
					return fs.SkipDir
				}

				return nil
			}

			target := filepath.Join(
				dst,
				rel,
			)

			if entry.IsDir() {
				if err := os.MkdirAll(
					target,
					0o755,
				); err != nil {
					return fmt.Errorf(
						"lab: create source directory %q: %w",
						target,
						err,
					)
				}

				return nil
			}

			info, err := entry.Info()
			if err != nil {
				return fmt.Errorf(
					"lab: inspect workspace entry %q: %w",
					path,
					err,
				)
			}

			// Never follow symlinks from the disposable workspace into the
			// user's source tree.
			if info.Mode()&os.ModeSymlink != 0 {
				return nil
			}

			if !info.Mode().IsRegular() {
				return nil
			}

			if err := copyFile(
				path,
				target,
				info.Mode().Perm(),
			); err != nil {
				return err
			}

			return nil
		},
	)
}

func copyFile(
	src string,
	dst string,
	perm os.FileMode,
) error {
	in, err := os.Open(src)
	if err != nil {
		return fmt.Errorf(
			"lab: open %q: %w",
			src,
			err,
		)
	}

	defer in.Close()

	if err := os.MkdirAll(
		filepath.Dir(dst),
		0o755,
	); err != nil {
		return fmt.Errorf(
			"lab: create parent for %q: %w",
			dst,
			err,
		)
	}

	out, err := os.OpenFile(
		dst,
		os.O_WRONLY|os.O_CREATE|os.O_TRUNC,
		perm,
	)
	if err != nil {
		return fmt.Errorf(
			"lab: create %q: %w",
			dst,
			err,
		)
	}

	_, copyErr := io.Copy(
		out,
		in,
	)

	closeErr := out.Close()

	if copyErr != nil {
		_ = os.Remove(dst)

		return fmt.Errorf(
			"lab: copy %q: %w",
			src,
			copyErr,
		)
	}

	if closeErr != nil {
		return fmt.Errorf(
			"lab: close %q: %w",
			dst,
			closeErr,
		)
	}

	return nil
}

func ensureDirectory(
	path string,
) error {
	info, err := os.Stat(path)
	if err != nil {
		return err
	}

	if !info.IsDir() {
		return fmt.Errorf(
			"path is not a directory: %s",
			path,
		)
	}

	return nil
}

func resolveExistingPrefix(
	path string,
) (string, error) {
	current := filepath.Clean(path)

	for {
		_, err := os.Lstat(current)

		if err == nil {
			real, evalErr := filepath.EvalSymlinks(
				current,
			)
			if evalErr != nil {
				return "", evalErr
			}

			abs, absErr := filepath.Abs(real)
			if absErr != nil {
				return "", absErr
			}

			return filepath.Clean(abs), nil
		}

		if !errors.Is(
			err,
			os.ErrNotExist,
		) {
			return "", err
		}

		parent := filepath.Dir(current)

		if parent == current {
			return "", ErrInvalidWorkspace
		}

		current = parent
	}
}

func newWorkspaceID() (string, error) {
	var b [8]byte

	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}

	return fmt.Sprintf(
		"task-%s-%s",
		time.Now().UTC().Format(
			"20060102-150405",
		),
		hex.EncodeToString(b[:]),
	), nil
}

func newSnapshotID() (string, error) {
	var b [8]byte

	if _, err := rand.Read(b[:]); err != nil {
		return "", err
	}

	return fmt.Sprintf(
		"snapshot-%s-%s",
		time.Now().UTC().Format(
			"20060102-150405",
		),
		hex.EncodeToString(b[:]),
	), nil
}

func sameOrWithin(
	path string,
	root string,
) bool {
	path = filepath.Clean(path)
	root = filepath.Clean(root)

	if path == root {
		return true
	}

	rel, err := filepath.Rel(
		root,
		path,
	)
	if err != nil {
		return false
	}

	return rel != ".." &&
		!strings.HasPrefix(
			rel,
			".."+string(filepath.Separator),
		) &&
		rel != ""
}
