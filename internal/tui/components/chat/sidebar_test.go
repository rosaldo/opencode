package chat

import (
	"testing"

	"github.com/opencode-ai/opencode/internal/config"
)

func TestSidebarClickZoneSessionOpensSwitcher(t *testing.T) {
	m := &sidebarCmp{}
	_, cmd := m.clickZone(sidebarZoneSession)
	if cmd == nil {
		t.Fatal("clicking the session title must open the switcher")
	}
	if _, ok := cmd().(ShowSessionListMsg); !ok {
		t.Fatalf("msg = %T, want ShowSessionListMsg", cmd())
	}
}

func TestSidebarClickZoneFileOpensEditor(t *testing.T) {
	t.Cleanup(func() { workingDirectory = config.WorkingDirectory })
	workingDirectory = func() string { return t.TempDir() }

	m := &sidebarCmp{}
	// The command wraps the editor process; it is only run by the Bubble Tea
	// runtime, so asserting the mapping is enough here.
	if _, cmd := m.clickZone(sidebarZoneFile + "src/main.go"); cmd == nil {
		t.Fatal("clicking a modified file must open it in the editor")
	}
}

func TestSidebarClickZoneUnknown(t *testing.T) {
	m := &sidebarCmp{}
	if _, cmd := m.clickZone("side:whatever"); cmd != nil {
		t.Error("unknown zones must be ignored")
	}
	if _, cmd := m.clickZone(sidebarZoneFile); cmd != nil {
		t.Error("the bare file prefix must be ignored")
	}
}
