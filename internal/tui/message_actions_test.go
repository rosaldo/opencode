package tui

import (
	"testing"

	"github.com/opencode-ai/opencode/internal/message"
	"github.com/opencode-ai/opencode/internal/snapshot"
)

func TestBoundarySnapshot(t *testing.T) {
	msgs := []message.Message{
		{Role: message.User, Parts: []message.ContentPart{message.SnapshotContent{SnapshotID: "tree-1"}}},
		{Role: message.Assistant},
		{Role: message.User, Parts: []message.ContentPart{message.SnapshotContent{SnapshotID: "tree-2"}}},
		{Role: message.Assistant},
	}

	cases := []struct {
		name string
		idx  int
		want string
	}{
		{"user message uses its own snapshot", 0, "tree-1"},
		{"assistant uses the preceding user snapshot", 1, "tree-1"},
		{"later user message", 2, "tree-2"},
		{"later assistant", 3, "tree-2"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := boundarySnapshot(msgs, tc.idx); got != tc.want {
				t.Errorf("boundarySnapshot(%d) = %q, want %q", tc.idx, got, tc.want)
			}
		})
	}

	plain := []message.Message{{Role: message.Assistant}}
	if got := boundarySnapshot(plain, 0); got != "" {
		t.Errorf("no user message = %q, want empty", got)
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
