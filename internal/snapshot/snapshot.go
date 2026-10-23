package snapshot

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/opencode-ai/opencode/internal/config"
	"github.com/opencode-ai/opencode/internal/git"
	"github.com/opencode-ai/opencode/internal/logging"
)

// Snapshot represents a captured state of the worktree.
type Snapshot struct {
	ID string
}

// FileDiff represents a file change between two snapshots.
type FileDiff struct {
	File      string
	Status    string // "A" = added, "M" = modified, "D" = deleted
	Additions int
	Deletions int
	Patch     string
}

// Service manages snapshots of the worktree using a shadow git repository.
type Service interface {
	// Enabled reports whether snapshots are active for the current worktree.
	Enabled() bool

	// Capture captures the current worktree state and returns a snapshot ID.
	// Returns nil if snapshots are disabled.
	Capture(ctx context.Context) (*Snapshot, error)

	// Restore restores files from a snapshot to the worktree.
	// paths is a list of relative paths to restore; if empty, restores all.
	Restore(ctx context.Context, snapshotID string, paths []string) error

	// RemoveFiles deletes the given worktree-relative paths (used to drop
	// files that did not exist in the target snapshot).
	RemoveFiles(ctx context.Context, paths []string) error

	// Diff returns the file changes between two snapshots.
	Diff(ctx context.Context, from, to string, paths []string) ([]FileDiff, error)

	// Files returns the list of files that differ between two snapshots.
	Files(ctx context.Context, from, to string) ([]string, error)
}

type service struct {
	mu sync.RWMutex

	// shadowRepo is the shadow git repository used for snapshots.
	shadowRepo *git.Repository

	// worktreeRepo is the user's real worktree (read-only for status checks).
	worktreeRepo *git.Repository

	// enabled indicates whether snapshots are active.
	enabled bool
}

// NewService creates a snapshot service for the given working directory.
// The shadow repository is stored under
// <global config dir>/snapshot/<projectHash>, outside the worktree.
func NewService(workingDir string) Service {
	s := &service{}

	// Check if the worktree is a git repository
	s.worktreeRepo = git.NewRepository(workingDir)
	ctx := context.Background()
	if !s.worktreeRepo.IsRepository(ctx) {
		s.enabled = false
		logging.Debug("Snapshots disabled: not a git repository")
		return s
	}

	// Check config
	cfg := config.Get()
	if cfg != nil && cfg.Snapshots != nil {
		s.enabled = *cfg.Snapshots
	} else {
		// Default: enabled
		s.enabled = true
	}

	if !s.enabled {
		logging.Debug("Snapshots disabled by config")
		return s
	}

	// Shadow repositories live under the global opencode directory (like the
	// TS snapshot service) so they never pollute or recurse into the worktree.
	absWorkdir, err := filepath.Abs(workingDir)
	if err != nil {
		absWorkdir = workingDir
	}
	shadowDir := filepath.Join(globalConfigDir(), "snapshot", hashString(absWorkdir))

	// Initialize shadow repository if needed
	s.shadowRepo = git.NewRepository(shadowDir)
	if !s.shadowRepo.IsRepository(ctx) {
		if err := os.MkdirAll(shadowDir, 0755); err != nil {
			logging.Warn("Failed to create shadow repo directory", "dir", shadowDir, "error", err)
			s.enabled = false
			return s
		}
		if _, stderr, err := s.shadowRepo.Run(ctx, "init"); err != nil {
			logging.Warn("Failed to initialize shadow repository", "dir", shadowDir, "error", err, "stderr", stderr)
			s.enabled = false
			return s
		}
		// Configure shadow repo for snapshot use
		if _, _, err := s.shadowRepo.Run(ctx, "config", "core.bare", "false"); err != nil {
			logging.Warn("Failed to configure shadow repository", "error", err)
		}
	}

	// Seed the shadow repository with the full worktree on first use so a
	// capture on a clean tree still knows every file's content.
	if empty, err := isDirEmpty(shadowDir); err != nil || empty {
		if seedErr := s.seed(ctx); seedErr != nil {
			logging.Warn("Failed to seed the snapshot repository", "error", seedErr)
			s.enabled = false
			return s
		}
	}

	logging.Debug("Snapshots enabled", "shadowDir", shadowDir)
	return s
}

// globalConfigDir returns the user-level opencode config directory
// (XDG_CONFIG_HOME/opencode or ~/.config/opencode).
func globalConfigDir() string {
	if xdg := os.Getenv("XDG_CONFIG_HOME"); xdg != "" {
		return filepath.Join(xdg, "opencode")
	}
	home, err := os.UserHomeDir()
	if err == nil && home != "" {
		return filepath.Join(home, ".config", "opencode")
	}
	return filepath.Join(os.TempDir(), "opencode")
}

// isDirEmpty reports whether dir contains no regular files (ignoring .git).
func isDirEmpty(dir string) (bool, error) {
	found := false
	err := filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil || found {
			return nil
		}
		if info.IsDir() && info.Name() == ".git" && path != dir {
			return filepath.SkipDir
		}
		if !info.IsDir() && path != dir {
			found = true
			return filepath.SkipAll
		}
		return nil
	})
	return !found, err
}

// seed copies every tracked/untracked (non-ignored) worktree file into the
// shadow repository and captures the initial tree.
func (s *service) seed(ctx context.Context) error {
	worktreeDir := s.worktreeRepo.Dir()
	shadowDir := s.shadowRepo.Dir()

	files, err := s.worktreeRepo.ListFiles(ctx)
	if err != nil {
		return fmt.Errorf("failed to list worktree files: %w", err)
	}
	for _, rel := range files {
		srcPath := filepath.Join(worktreeDir, rel)
		data, err := os.ReadFile(srcPath)
		if err != nil {
			logging.Debug("Failed to read file while seeding snapshots", "path", rel, "error", err)
			continue
		}
		dstPath := filepath.Join(shadowDir, rel)
		if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
			return fmt.Errorf("failed to create shadow directory: %w", err)
		}
		if err := os.WriteFile(dstPath, data, 0644); err != nil {
			return fmt.Errorf("failed to write shadow file: %w", err)
		}
	}
	// Capture once so the seeded files are staged in the shadow index; the
	// tree itself is refreshed by every Capture call.
	if _, err := s.shadowRepo.CaptureTree(ctx); err != nil {
		return fmt.Errorf("failed to capture the seeded tree: %w", err)
	}
	return nil
}

func (s *service) Enabled() bool {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.enabled
}

func (s *service) Capture(ctx context.Context) (*Snapshot, error) {
	s.mu.RLock()
	if !s.enabled {
		s.mu.RUnlock()
		return nil, nil
	}
	s.mu.RUnlock()

	// Sync worktree files to shadow repo and capture tree
	treeHash, err := s.syncAndCapture(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to capture snapshot: %w", err)
	}

	return &Snapshot{ID: treeHash}, nil
}

func (s *service) Restore(ctx context.Context, snapshotID string, paths []string) error {
	s.mu.RLock()
	if !s.enabled {
		s.mu.RUnlock()
		return fmt.Errorf("snapshots are disabled")
	}
	shadowRepo := s.shadowRepo
	worktreeDir := s.worktreeRepo.Dir()
	s.mu.RUnlock()

	// Restore files from shadow repo to worktree
	// We need to use a temporary worktree or copy files directly
	return restoreToWorktree(ctx, shadowRepo, worktreeDir, snapshotID, paths)
}

func (s *service) Diff(ctx context.Context, from, to string, paths []string) ([]FileDiff, error) {
	s.mu.RLock()
	if !s.enabled {
		s.mu.RUnlock()
		return nil, fmt.Errorf("snapshots are disabled")
	}
	shadowRepo := s.shadowRepo
	s.mu.RUnlock()

	gitDiffs, err := shadowRepo.DiffTrees(ctx, from, to, paths)
	if err != nil {
		return nil, err
	}

	diffs := make([]FileDiff, len(gitDiffs))
	for i, d := range gitDiffs {
		diffs[i] = FileDiff{
			File:      d.File,
			Status:    d.Status,
			Additions: d.Additions,
			Deletions: d.Deletions,
			Patch:     d.Patch,
		}
	}
	return diffs, nil
}

func (s *service) Files(ctx context.Context, from, to string) ([]string, error) {
	s.mu.RLock()
	if !s.enabled {
		s.mu.RUnlock()
		return nil, fmt.Errorf("snapshots are disabled")
	}
	shadowRepo := s.shadowRepo
	s.mu.RUnlock()

	diffs, err := shadowRepo.DiffTrees(ctx, from, to, nil)
	if err != nil {
		return nil, err
	}

	files := make([]string, len(diffs))
	for i, d := range diffs {
		files[i] = d.File
	}
	return files, nil
}

func (s *service) RemoveFiles(ctx context.Context, paths []string) error {
	s.mu.RLock()
	if !s.enabled {
		s.mu.RUnlock()
		return fmt.Errorf("snapshots are disabled")
	}
	worktreeDir := s.worktreeRepo.Dir()
	s.mu.RUnlock()

	for _, rel := range paths {
		clean := filepath.Clean(rel)
		if clean == "." || strings.HasPrefix(clean, ".."+string(filepath.Separator)) || filepath.IsAbs(clean) {
			continue
		}
		if err := os.Remove(filepath.Join(worktreeDir, clean)); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("failed to remove %s: %w", clean, err)
		}
	}
	return nil
}

// syncAndCapture copies worktree files to the shadow repo and captures the tree.
func (s *service) syncAndCapture(ctx context.Context) (string, error) {
	worktreeDir := s.worktreeRepo.Dir()
	shadowDir := s.shadowRepo.Dir()

	// Sync the files reported by git status into the shadow repository.
	changed, err := s.worktreeRepo.ChangedFiles(ctx)
	if err != nil {
		return "", fmt.Errorf("failed to get worktree status: %w", err)
	}

	for _, file := range changed {
		status := file.Status
		path := file.Path

		// Skip deleted files (they must vanish from the shadow worktree).
		if status[0] == 'D' || status[1] == 'D' {
			shadowPath := filepath.Join(shadowDir, path)
			os.Remove(shadowPath)
			continue
		}

		// Copy file from worktree to shadow
		srcPath := filepath.Join(worktreeDir, path)
		dstPath := filepath.Join(shadowDir, path)

		// Ensure destination directory exists
		if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
			return "", fmt.Errorf("failed to create directory: %w", err)
		}

		// Copy file content
		data, err := os.ReadFile(srcPath)
		if err != nil {
			// File might be binary or unreadable; skip
			logging.Debug("Failed to read file for snapshot", "path", path, "error", err)
			continue
		}
		if err := os.WriteFile(dstPath, data, 0644); err != nil {
			return "", fmt.Errorf("failed to write file to shadow: %w", err)
		}
	}

	// Capture tree in shadow repo
	return s.shadowRepo.CaptureTree(ctx)
}

// restoreToWorktree restores files from a snapshot in the shadow repo to the worktree.
func restoreToWorktree(ctx context.Context, shadowRepo *git.Repository, worktreeDir, snapshotID string, paths []string) error {
	// Get list of files in the snapshot
	// We'll read-tree into a temp index, then copy files
	tmpIndex := filepath.Join(os.TempDir(), fmt.Sprintf("snapshot-index-%d", os.Getpid()))
	defer os.Remove(tmpIndex)

	// Use GIT_INDEX_FILE environment to avoid touching the real index
	// This is a limitation of the current git wrapper; we'll use a simpler approach:
	// Just copy files from shadow repo worktree

	// First, ensure the shadow repo has the snapshot checked out
	// We read-tree into the shadow repo's index, then checkout-index to a temp dir
	if _, stderr, err := shadowRepo.Run(ctx, "read-tree", snapshotID); err != nil {
		return fmt.Errorf("failed to read tree: %w: %s", err, stderr)
	}

	// Checkout files to a temp directory
	tmpDir, err := os.MkdirTemp("", "snapshot-restore-*")
	if err != nil {
		return fmt.Errorf("failed to create temp dir: %w", err)
	}
	defer os.RemoveAll(tmpDir)

	if len(paths) == 0 {
		if _, stderr, err := shadowRepo.Run(ctx, "checkout-index", "-a", "--prefix="+tmpDir+"/"); err != nil {
			return fmt.Errorf("failed to checkout index: %w: %s", err, stderr)
		}
	} else {
		args := append([]string{"checkout-index", "--prefix=" + tmpDir + "/", "--"}, paths...)
		if _, stderr, err := shadowRepo.Run(ctx, args...); err != nil {
			return fmt.Errorf("failed to checkout index: %w: %s", err, stderr)
		}
	}

	// Copy files from temp dir to worktree
	return copyDir(tmpDir, worktreeDir)
}

// copyDir recursively copies files from src to dst.
func copyDir(src, dst string) error {
	return filepath.Walk(src, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			return nil
		}
		relPath, err := filepath.Rel(src, path)
		if err != nil {
			return err
		}
		dstPath := filepath.Join(dst, relPath)
		if err := os.MkdirAll(filepath.Dir(dstPath), 0755); err != nil {
			return err
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		return os.WriteFile(dstPath, data, info.Mode())
	})
}

// hashString returns the hex-encoded SHA-256 of a string.
func hashString(s string) string {
	h := sha256.Sum256([]byte(s))
	return hex.EncodeToString(h[:])[:16]
}
