package git

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

func TestRepository(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()

	// Initialize a git repo
	r := NewRepository(dir)
	if !r.IsRepository(ctx) {
		// Init a new repo
		if _, _, err := r.run(ctx, "init"); err != nil {
			t.Fatalf("failed to init repo: %v", err)
		}
		if !r.IsRepository(ctx) {
			t.Fatal("repo should be initialized")
		}
	}

	// Create a file
	testFile := filepath.Join(dir, "test.txt")
	if err := os.WriteFile(testFile, []byte("hello\n"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	// Capture tree
	tree1, err := r.CaptureTree(ctx)
	if err != nil {
		t.Fatalf("failed to capture tree: %v", err)
	}
	if tree1 == "" {
		t.Fatal("tree hash should not be empty")
	}

	// Modify the file
	if err := os.WriteFile(testFile, []byte("hello\nworld\n"), 0644); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}

	// Capture again
	tree2, err := r.CaptureTree(ctx)
	if err != nil {
		t.Fatalf("failed to capture tree: %v", err)
	}

	// Diff should show modification
	diffs, err := r.DiffTrees(ctx, tree1, tree2, nil)
	if err != nil {
		t.Fatalf("failed to diff trees: %v", err)
	}
	if len(diffs) != 1 {
		t.Fatalf("expected 1 diff, got %d", len(diffs))
	}
	if diffs[0].File != "test.txt" {
		t.Errorf("expected test.txt, got %s", diffs[0].File)
	}
	if diffs[0].Status != "M" {
		t.Errorf("expected M status, got %s", diffs[0].Status)
	}

	// Restore from tree1
	if err := r.RestoreTree(ctx, tree1, nil); err != nil {
		t.Fatalf("failed to restore tree: %v", err)
	}

	// File should be back to original
	content, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatalf("failed to read file: %v", err)
	}
	if string(content) != "hello\n" {
		t.Errorf("expected 'hello\\n', got %q", string(content))
	}
}
