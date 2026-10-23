package snapshot

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/opencode-ai/opencode/internal/git"
)

func initRepo(t *testing.T, dir string) *git.Repository {
	t.Helper()
	ctx := context.Background()
	r := git.NewRepository(dir)
	if _, stderr, err := r.Run(ctx, "init"); err != nil {
		t.Fatalf("failed to init git repo: %v: %s", err, stderr)
	}
	if _, _, err := r.Run(ctx, "config", "user.email", "test@test.com"); err != nil {
		t.Fatalf("failed to config user.email: %v", err)
	}
	if _, _, err := r.Run(ctx, "config", "user.name", "Test"); err != nil {
		t.Fatalf("failed to config user.name: %v", err)
	}
	return r
}

func TestSnapshotService(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	ctx := context.Background()

	worktreeDir := t.TempDir()
	initRepo(t, worktreeDir)

	svc := NewService(worktreeDir)
	if !svc.Enabled() {
		t.Fatal("service should be enabled for a git repo")
	}

	testFile := filepath.Join(worktreeDir, "test.txt")
	if err := os.WriteFile(testFile, []byte("hello\n"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	snap1, err := svc.Capture(ctx)
	if err != nil {
		t.Fatalf("failed to capture: %v", err)
	}
	if snap1 == nil {
		t.Fatal("snapshot should not be nil")
	}

	if err := os.WriteFile(testFile, []byte("hello\nworld\n"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	snap2, err := svc.Capture(ctx)
	if err != nil {
		t.Fatalf("failed to capture: %v", err)
	}

	diffs, err := svc.Diff(ctx, snap1.ID, snap2.ID, nil)
	if err != nil {
		t.Fatalf("failed to diff: %v", err)
	}
	if len(diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(diffs))
	}
	if diffs[0].File != "test.txt" {
		t.Errorf("expected test.txt, got %s", diffs[0].File)
	}

	files, err := svc.Files(ctx, snap1.ID, snap2.ID)
	if err != nil {
		t.Fatalf("failed to get files: %v", err)
	}
	if len(files) != 1 || files[0] != "test.txt" {
		t.Errorf("expected [test.txt], got %v", files)
	}

	if err := svc.Restore(ctx, snap1.ID, nil); err != nil {
		t.Fatalf("failed to restore: %v", err)
	}

	content, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	if string(content) != "hello\n" {
		t.Errorf("expected 'hello\\n', got %q", string(content))
	}
}

// TestSnapshotSeededCleanTree covers the common case: the worktree is clean
// when opencode starts, so only the seed provides the pre-turn file content.
func TestSnapshotSeededCleanTree(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	ctx := context.Background()

	worktreeDir := t.TempDir()
	r := initRepo(t, worktreeDir)

	testFile := filepath.Join(worktreeDir, "a.txt")
	if err := os.WriteFile(testFile, []byte("v1\n"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}
	if _, _, err := r.Run(ctx, "add", "-A"); err != nil {
		t.Fatalf("failed to add: %v", err)
	}
	if _, stderr, err := r.Run(ctx, "commit", "-m", "init"); err != nil {
		t.Fatalf("failed to commit: %v: %s", err, stderr)
	}

	// Service creation seeds the shadow repo from the (clean) worktree.
	svc := NewService(worktreeDir)
	before, err := svc.Capture(ctx)
	if err != nil || before == nil {
		t.Fatalf("failed to capture clean tree: %v", err)
	}

	if err := os.WriteFile(testFile, []byte("v2\n"), 0644); err != nil {
		t.Fatalf("failed to modify file: %v", err)
	}
	after, err := svc.Capture(ctx)
	if err != nil {
		t.Fatalf("failed to capture: %v", err)
	}

	diffs, err := svc.Diff(ctx, before.ID, after.ID, nil)
	if err != nil {
		t.Fatalf("failed to diff: %v", err)
	}
	if len(diffs) != 1 || diffs[0].File != "a.txt" || diffs[0].Status != "M" {
		t.Fatalf("diffs = %+v, want a.txt modified", diffs)
	}

	if err := svc.Restore(ctx, before.ID, nil); err != nil {
		t.Fatalf("failed to restore: %v", err)
	}
	content, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	if string(content) != "v1\n" {
		t.Errorf("expected 'v1\\n', got %q", string(content))
	}
}

func TestSnapshotCreatedFilesAppearAsDeletedInReverseDiff(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	ctx := context.Background()

	worktreeDir := t.TempDir()
	initRepo(t, worktreeDir)
	svc := NewService(worktreeDir)

	before, err := svc.Capture(ctx)
	if err != nil {
		t.Fatalf("failed to capture: %v", err)
	}

	created := filepath.Join(worktreeDir, "created.txt")
	if err := os.WriteFile(created, []byte("new\n"), 0644); err != nil {
		t.Fatalf("failed to create file: %v", err)
	}
	after, err := svc.Capture(ctx)
	if err != nil {
		t.Fatalf("failed to capture: %v", err)
	}

	// Reverse diff (after -> before): the created file is "D".
	diffs, err := svc.Diff(ctx, after.ID, before.ID, nil)
	if err != nil {
		t.Fatalf("failed to diff: %v", err)
	}
	if len(diffs) != 1 || diffs[0].Status != "D" {
		t.Fatalf("diffs = %+v, want created.txt deleted in reverse diff", diffs)
	}

	if err := svc.RemoveFiles(ctx, []string{"created.txt"}); err != nil {
		t.Fatalf("failed to remove: %v", err)
	}
	if _, err := os.Stat(created); !os.IsNotExist(err) {
		t.Errorf("created.txt should have been removed, stat err = %v", err)
	}
}

func TestSnapshotDisabledForNonGitRepo(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", t.TempDir())
	dir := t.TempDir()
	svc := NewService(dir)
	if svc.Enabled() {
		t.Fatal("service should be disabled for non-git repo")
	}
}
