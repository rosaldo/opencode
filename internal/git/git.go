package git

import (
	"bytes"
	"context"
	"fmt"
	"os/exec"
	"strings"
)

// Repository wraps git operations for a specific repository directory.
type Repository struct {
	dir string
}

// NewRepository creates a Repository for the given directory.
func NewRepository(dir string) *Repository {
	return &Repository{dir: dir}
}

// Dir returns the repository directory.
func (r *Repository) Dir() string {
	return r.dir
}

// run executes a git command and returns stdout, stderr, and error.
func (r *Repository) run(ctx context.Context, args ...string) (string, string, error) {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = r.dir
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

// Run executes a git command and returns stdout, stderr, and error.
// Exported for use by the snapshot service.
func (r *Repository) Run(ctx context.Context, args ...string) (string, string, error) {
	return r.run(ctx, args...)
}

// IsRepository checks if the directory is a git repository.
func (r *Repository) IsRepository(ctx context.Context) bool {
	_, _, err := r.run(ctx, "rev-parse", "--git-dir")
	return err == nil
}

// CaptureTree stages all changes and creates a commit, returning the tree hash.
// This is used by the snapshot service to capture the current worktree state.
func (r *Repository) CaptureTree(ctx context.Context) (string, error) {
	// Add all files (including untracked, excluding ignored)
	if _, stderr, err := r.run(ctx, "add", "-A"); err != nil {
		return "", fmt.Errorf("git add failed: %w: %s", err, stderr)
	}

	// Create a commit and capture its tree hash
	// Using a detached HEAD commit to avoid polluting the branch history
	treeOut, stderr, err := r.run(ctx, "write-tree")
	if err != nil {
		return "", fmt.Errorf("git write-tree failed: %w: %s", err, stderr)
	}
	return strings.TrimSpace(treeOut), nil
}

// RestoreTree restores files from a tree hash to the worktree.
// paths is a list of relative paths to restore; if empty, restores all.
func (r *Repository) RestoreTree(ctx context.Context, treeHash string, paths []string) error {
	// checkout-index with --all reads from the index, not a tree.
	// We need to first update the index from the tree, then checkout.
	args := []string{"read-tree", treeHash}
	if _, stderr, err := r.run(ctx, args...); err != nil {
		return fmt.Errorf("git read-tree failed: %w: %s", err, stderr)
	}

	if len(paths) == 0 {
		// Restore all files from the index
		if _, stderr, err := r.run(ctx, "checkout-index", "-a", "-f"); err != nil {
			return fmt.Errorf("git checkout-index failed: %w: %s", err, stderr)
		}
	} else {
		// Restore specific files
		args := append([]string{"checkout-index", "-f", "--"}, paths...)
		if _, stderr, err := r.run(ctx, args...); err != nil {
			return fmt.Errorf("git checkout-index failed: %w: %s", err, stderr)
		}
	}
	return nil
}

// DiffTrees returns the list of files changed between two trees, along with
// their status (added, modified, deleted) and diff content.
func (r *Repository) DiffTrees(ctx context.Context, from, to string, paths []string) ([]FileDiff, error) {
	args := []string{"diff", "--name-status", from, to}
	if len(paths) > 0 {
		args = append(args, "--")
		args = append(args, paths...)
	}
	nameStatusOut, stderr, err := r.run(ctx, args...)
	if err != nil {
		return nil, fmt.Errorf("git diff --name-status failed: %w: %s", err, stderr)
	}

	// Get the full diff for these paths
	diffArgs := []string{"diff", from, to}
	if len(paths) > 0 {
		diffArgs = append(diffArgs, "--")
		diffArgs = append(diffArgs, paths...)
	}
	diffOut, stderr, err := r.run(ctx, diffArgs...)
	if err != nil {
		return nil, fmt.Errorf("git diff failed: %w: %s", err, stderr)
	}

	return parseDiff(nameStatusOut, diffOut), nil
}

// FileDiff represents a file change between two trees.
type FileDiff struct {
	File      string
	Status    string // "A" = added, "M" = modified, "D" = deleted
	Additions int
	Deletions int
	Patch     string
}

// parseDiff parses git diff --name-status and git diff output.
func parseDiff(nameStatus, fullDiff string) []FileDiff {
	diffs := make(map[string]*FileDiff)

	// Parse name-status
	for _, line := range strings.Split(nameStatus, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		parts := strings.SplitN(line, "\t", 2)
		if len(parts) != 2 {
			continue
		}
		status := parts[0]
		file := parts[1]
		// Handle renames (R100\told\tnew)
		if strings.HasPrefix(status, "R") {
			pathParts := strings.SplitN(parts[1], "\t", 2)
			if len(pathParts) == 2 {
				file = pathParts[1] // new path
			}
		}
		diffs[file] = &FileDiff{
			File:   file,
			Status: status[:1],
		}
	}

	// Parse full diff to extract patches and count additions/deletions
	currentFile := ""
	for _, line := range strings.Split(fullDiff, "\n") {
		if strings.HasPrefix(line, "+++ b/") {
			currentFile = strings.TrimPrefix(line, "+++ b/")
			if d, ok := diffs[currentFile]; ok {
				d.Patch = line + "\n"
			}
		} else if strings.HasPrefix(line, "--- a/") {
			// Skip
		} else if currentFile != "" {
			if d, ok := diffs[currentFile]; ok {
				d.Patch += line + "\n"
				if strings.HasPrefix(line, "+") && !strings.HasPrefix(line, "+++") {
					d.Additions++
				} else if strings.HasPrefix(line, "-") && !strings.HasPrefix(line, "---") {
					d.Deletions++
				}
			}
		}
	}

	// Convert map to slice in stable order
	result := make([]FileDiff, 0, len(diffs))
	for _, d := range diffs {
		result = append(result, *d)
	}
	return result
}

// WorktreeStatus returns the current status of the worktree (modified files).
func (r *Repository) WorktreeStatus(ctx context.Context) (string, error) {
	out, stderr, err := r.run(ctx, "status", "--porcelain")
	if err != nil {
		return "", fmt.Errorf("git status failed: %w: %s", err, stderr)
	}
	return out, nil
}

// ListFiles returns every tracked or untracked-but-not-ignored file in the
// worktree, relative to the repository root.
func (r *Repository) ListFiles(ctx context.Context) ([]string, error) {
	out, stderr, err := r.run(ctx, "ls-files", "-z", "--cached", "--others", "--exclude-standard")
	if err != nil {
		return nil, fmt.Errorf("git ls-files failed: %w: %s", err, stderr)
	}
	files := make([]string, 0)
	for _, f := range strings.Split(out, "\x00") {
		if f != "" {
			files = append(files, f)
		}
	}
	return files, nil
}
