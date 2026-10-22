package tui

import (
	"reflect"
	"testing"
)

func TestResolveKeybindsDefaults(t *testing.T) {
	kb, warnings := ResolveKeybinds(nil)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if kb.Leader != defaultLeader {
		t.Fatalf("leader = %q, want %q", kb.Leader, defaultLeader)
	}
	if got := kb.Keys(KeybindSessionList); !reflect.DeepEqual(got, []string{"ctrl+s"}) {
		t.Fatalf("session_list = %v", got)
	}
	// Defaults must equal the pre-Spec-09 keymap.
	cases := map[KeybindID][]string{
		KeybindAppExit:         {"ctrl+c"},
		KeybindCommandList:     {"ctrl+k"},
		KeybindAgentList:       {"tab"},
		KeybindLogsToggle:      {"ctrl+l"},
		KeybindFilePicker:      {"ctrl+f"},
		KeybindModelList:       {"ctrl+o"},
		KeybindThemeList:       {"ctrl+t"},
		KeybindHelpToggle:      {"ctrl+_"},
		KeybindSessionNew:      {"ctrl+x", "n"},
		KeybindSessionRename:   {"ctrl+x", "r"},
		KeybindSessionDelete:   {"ctrl+x", "d"},
		KeybindSessionTimeline: {"ctrl+x", "g"},
	}
	for id, want := range cases {
		if got := kb.Keys(id); !reflect.DeepEqual(got, want) {
			t.Errorf("%s = %v, want %v", id, got, want)
		}
	}
	// session_copy is palette-only, like the TS default (Spec 09o).
	if kb.Enabled(KeybindSessionCopy) {
		t.Error("session_copy default must be disabled")
	}
}

func TestResolveKeybindsOverride(t *testing.T) {
	kb, warnings := ResolveKeybinds(map[string]string{
		"session_delete": "ctrl+d",
		"agent_list":     "ctrl+a,tab",
	})
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if got := kb.Keys(KeybindSessionDelete); !reflect.DeepEqual(got, []string{"ctrl+d"}) {
		t.Fatalf("session_delete = %v", got)
	}
	if got := kb.Keys(KeybindAgentList); !reflect.DeepEqual(got, []string{"ctrl+a", "tab"}) {
		t.Fatalf("agent_list = %v", got)
	}
}

func TestResolveKeybindsNoneDisables(t *testing.T) {
	for _, v := range []string{"none", "NONE", "false", ""} {
		kb, warnings := ResolveKeybinds(map[string]string{"session_list": v})
		if len(warnings) != 0 {
			t.Fatalf("value %q: unexpected warnings: %v", v, warnings)
		}
		if kb.Enabled(KeybindSessionList) {
			t.Errorf("value %q: session_list should be disabled", v)
		}
	}
}

func TestResolveKeybindsLeader(t *testing.T) {
	kb, warnings := ResolveKeybinds(map[string]string{"leader": "ctrl+a"})
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	if kb.Leader != "ctrl+a" {
		t.Fatalf("leader = %q", kb.Leader)
	}
	if got := kb.Keys(KeybindSessionDelete); !reflect.DeepEqual(got, []string{"ctrl+a", "d"}) {
		t.Fatalf("session_delete = %v", got)
	}
}

func TestResolveKeybindsUnknown(t *testing.T) {
	kb, warnings := ResolveKeybinds(map[string]string{"does_not_exist": "ctrl+z"})
	if len(warnings) != 1 {
		t.Fatalf("want 1 warning, got %v", warnings)
	}
	// Unknown id is ignored and defaults still apply.
	if !kb.Enabled(KeybindSessionList) {
		t.Fatal("defaults must still resolve")
	}
}

func TestResolveKeybindsLeaderCannotBeDisabled(t *testing.T) {
	_, warnings := ResolveKeybinds(map[string]string{"leader": "none"})
	if len(warnings) != 1 {
		t.Fatalf("want 1 warning, got %v", warnings)
	}
}

func TestResolveKeybindsUnknownDoesNotDisableOthers(t *testing.T) {
	kb, _ := ResolveKeybinds(map[string]string{"nope": "ctrl+z", "session_list": "ctrl+y"})
	if got := kb.Keys(KeybindSessionList); !reflect.DeepEqual(got, []string{"ctrl+y"}) {
		t.Fatalf("session_list = %v", got)
	}
}

func TestApplyLeaderBindingsDefaults(t *testing.T) {
	restore := resolvedKeybinds
	defer func() { resolvedKeybinds = restore }()

	kb, warnings := ResolveKeybinds(nil)
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	resolvedKeybinds = kb
	applyLeaderBindings()

	want := map[string]leaderAction{
		"n": leaderNewSession,
		"r": leaderRenameSession,
		"d": leaderDeleteSession,
		"c": leaderCompactSession,
		"b": leaderToggleSidebar,
		"x": leaderExportSession,
		"y": leaderCopyMessage,
		"u": leaderUndoMessage,
		".": leaderRedoMessage,
		"e": leaderOpenEditor,
		"g": leaderTimeline,
	}
	if !reflect.DeepEqual(leaderSuffixes, want) {
		t.Errorf("suffixes = %v, want %v", leaderSuffixes, want)
	}
	if len(leaderDirect) != 0 {
		t.Errorf("direct = %v, want none with the default leader", leaderDirect)
	}
}

func TestApplyLeaderBindingsReconfigured(t *testing.T) {
	restore := resolvedKeybinds
	defer func() { resolvedKeybinds = restore }()

	kb, warnings := ResolveKeybinds(map[string]string{
		"leader":          "ctrl+a",
		"session_delete":  "ctrl+d",
		"session_compact": "none",
	})
	if len(warnings) != 0 {
		t.Fatalf("unexpected warnings: %v", warnings)
	}
	resolvedKeybinds = kb
	applyLeaderBindings()

	// The leader key follows the configured "leader" keybind.
	want := map[string]leaderAction{
		"n": leaderNewSession,
		"r": leaderRenameSession,
		"b": leaderToggleSidebar,
		"x": leaderExportSession,
		"y": leaderCopyMessage,
		"u": leaderUndoMessage,
		".": leaderRedoMessage,
		"e": leaderOpenEditor,
		"g": leaderTimeline,
	}
	if !reflect.DeepEqual(leaderSuffixes, want) {
		t.Errorf("suffixes = %v, want %v", leaderSuffixes, want)
	}
	if _, ok := leaderSuffixes["d"]; ok {
		t.Error("session_delete bound without the leader must not be a suffix")
	}
	if _, ok := leaderSuffixes["c"]; ok {
		t.Error("session_compact disabled must not register a suffix")
	}

	binding, ok := leaderDirect[leaderDeleteSession]
	if !ok {
		t.Fatal("session_delete must be registered as a direct binding")
	}
	if got := binding.Keys(); !reflect.DeepEqual(got, []string{"ctrl+d"}) {
		t.Errorf("direct session_delete = %v, want [ctrl+d]", got)
	}
	if _, ok := leaderDirect[leaderCompactSession]; ok {
		t.Error("disabled session_compact must not register a direct binding")
	}
}
