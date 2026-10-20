package dialog

import (
	"testing"

	"github.com/opencode-ai/opencode/internal/agentdef"
)

func newFilterFixture() *agentDialogCmp {
	return &agentDialogCmp{
		agents: []agentdef.Definition{
			{Name: "coder", Description: "general purpose coding agent"},
			{Name: "security-audit", Description: "reviews code for vulnerabilities"},
			{Name: "docs-writer", Description: "writes documentation"},
			{Name: "reviewer", Description: "code review specialist"},
		},
	}
}

func filteredNames(m *agentDialogCmp) []string {
	out := make([]string, 0, len(m.filteredIdx))
	for _, idx := range m.filteredIdx {
		out = append(out, m.agents[idx].Name)
	}
	return out
}

func TestApplyFilterEmptyQueryKeepsAll(t *testing.T) {
	m := newFilterFixture()
	m.applyFilter()

	if len(m.filteredIdx) != 4 {
		t.Fatalf("filtered %d agents, want 4", len(m.filteredIdx))
	}
}

func TestApplyFilterMatchesName(t *testing.T) {
	m := newFilterFixture()
	m.input.SetValue("security")
	m.applyFilter()

	got := filteredNames(m)
	if len(got) != 1 || got[0] != "security-audit" {
		t.Fatalf("filter %q = %v, want [security-audit]", m.input.Value(), got)
	}
}

func TestApplyFilterMatchesDescription(t *testing.T) {
	m := newFilterFixture()
	m.input.SetValue("documentation")
	m.applyFilter()

	got := filteredNames(m)
	if len(got) != 1 || got[0] != "docs-writer" {
		t.Fatalf("filter %q = %v, want [docs-writer]", m.input.Value(), got)
	}
}

func TestApplyFilterIsCaseInsensitive(t *testing.T) {
	m := newFilterFixture()
	m.input.SetValue("REVIEW")
	m.applyFilter()

	got := filteredNames(m)
	if len(got) != 2 {
		t.Fatalf("filter %q = %v, want security-audit and reviewer", m.input.Value(), got)
	}
	want := map[string]bool{"security-audit": true, "reviewer": true}
	for _, name := range got {
		if !want[name] {
			t.Fatalf("unexpected match %q", name)
		}
	}
}

func TestApplyFilterRanksNameMatchesFirst(t *testing.T) {
	m := newFilterFixture()
	m.input.SetValue("review")
	m.applyFilter()

	got := filteredNames(m)
	if len(got) != 2 || got[0] != "reviewer" {
		t.Fatalf("filter %q = %v, want reviewer ranked first", m.input.Value(), got)
	}
}

func TestApplyFilterNoMatch(t *testing.T) {
	m := newFilterFixture()
	m.input.SetValue("zzzzz")
	m.applyFilter()

	if len(m.filteredIdx) != 0 {
		t.Fatalf("filter %q = %v, want empty", m.input.Value(), filteredNames(m))
	}
}

func TestApplyFilterResetsSelectionBounds(t *testing.T) {
	m := newFilterFixture()
	m.input.SetValue("review")
	m.applyFilter()
	m.selectedIdx = len(m.filteredIdx) - 1

	m.input.SetValue("zzzzz")
	m.applyFilter()

	if m.selectedIdx != 0 {
		t.Fatalf("selectedIdx = %d after narrowing to empty, want 0", m.selectedIdx)
	}
}
