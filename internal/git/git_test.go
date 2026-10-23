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

func TestChangedFilesHandlesSpacesAndRenames(t *testing.T) {
	ctx := context.Background()
	dir := t.TempDir()
	r := NewRepository(dir)
	if _, stderr, err := r.Run(ctx, "init"); err != nil {
		t.Fatalf("failed to init: %v: %s", err, stderr)
	}
	if _, _, err := r.Run(ctx, "config", "user.email", "t@t.com"); err != nil {
		t.Fatalf("config email: %v", err)
	}
	if _, _, err := r.Run(ctx, "config", "user.name", "T"); err != nil {
		t.Fatalf("config name: %v", err)
	}

	original := filepath.Join(dir, "original name.txt")
	if err := os.WriteFile(original, []byte("content\n"), 0644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	if _, _, err := r.Run(ctx, "add", "-A"); err != nil {
		t.Fatalf("add: %v", err)
	}
	if _, stderr, err := r.Run(ctx, "commit", "-m", "init"); err != nil {
		t.Fatalf("commit: %v: %s", err, stderr)
	}

	if _, _, err := r.Run(ctx, "mv", "original name.txt", "renamed name.txt"); err != nil {
		t.Fatalf("mv: %v", err)
	}
	untracked := filepath.Join(dir, "novo arquivo.txt")
	if err := os.WriteFile(untracked, []byte("x\n"), 0644); err != nil {
		t.Fatalf("write untracked: %v", err)
	}

	files, err := r.ChangedFiles(ctx)
	if err != nil {
		t.Fatalf("ChangedFiles: %v", err)
	}

	paths := map[string]string{}
	for _, f := range files {
		paths[f.Path] = f.Status
	}
	if status, ok := paths["renamed name.txt"]; !ok || status[0] != 'R' {
		t.Errorf("renamed path missing or wrong status: %+v", files)
	}
	if status, ok := paths["novo arquivo.txt"]; !ok || status != "??" {
		t.Errorf("untracked path missing or wrong status: %+v", files)
	}
	if _, ok := paths["original name.txt"]; ok {
		t.Errorf("original rename path should be skipped, got %+v", files)
	}
}
