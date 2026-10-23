package tui

import (
	"testing"

	"github.com/opencode-ai/opencode/internal/message"
	"github.com/opencode-ai/opencode/internal/snapshot"
)

func TestBoundarySnapshot(t *testing.T) {
	msgs := []message.Message{
		{Role: message.User, Parts: []message.ContentPart{message.SnapshotContent{SnapshotID: "tree-1", Directory: "/one"}}},
		{Role: message.Assistant},
		{Role: message.User, Parts: []message.ContentPart{message.SnapshotContent{SnapshotID: "tree-2", Directory: "/two"}}},
		{Role: message.Assistant},
	}

	cases := []struct {
		name    string
		idx     int
		wantID  string
		wantDir string
	}{
		{"user message uses its own snapshot", 0, "tree-1", "/one"},
		{"assistant uses the preceding user snapshot", 1, "tree-1", "/one"},
		{"later user message", 2, "tree-2", "/two"},
		{"later assistant", 3, "tree-2", "/two"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, dir := boundarySnapshot(msgs, tc.idx)
			if got != tc.wantID || dir != tc.wantDir {
				t.Errorf("boundarySnapshot(%d) = (%q, %q), want (%q, %q)", tc.idx, got, dir, tc.wantID, tc.wantDir)
			}
		})
	}

	plain := []message.Message{{Role: message.Assistant}}
	if id, dir := boundarySnapshot(plain, 0); id != "" || dir != "" {
		t.Errorf("no user message = (%q, %q), want empty", id, dir)
	}
}

func TestStagedRevertBannerText(t *testing.T) {
	cases := []struct {
		name  string
		count int
		files []snapshot.FileDiff
		want  string
	}{
		{"single message", 1, nil, "◇ 1 message reverted · redo restores"},
		{"multiple messages", 3, nil, "◇ 3 messages reverted · redo restores"},
		{"single file", 1, []snapshot.FileDiff{{File: "a.txt"}}, "◇ 1 message reverted · 1 file rolled back · redo restores"},
		{"multiple files", 2, []snapshot.FileDiff{{File: "a.txt"}, {File: "b.txt"}}, "◇ 2 messages reverted · 2 files rolled back · redo restores"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			r := &stagedRevert{count: tc.count, files: tc.files}
			if got := r.bannerText(); got != tc.want {
				t.Errorf("bannerText() = %q, want %q", got, tc.want)
			}
		})
	}
}
