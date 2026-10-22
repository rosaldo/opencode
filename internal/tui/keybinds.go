package tui

import (
	"fmt"
	"sort"
	"strings"
)

// KeybindID identifies a configurable TUI action (Spec 09). The set is a
// deliberate subset of the upstream opencode keybinds.
type KeybindID string

const (
	KeybindAppExit        KeybindID = "app_exit"
	KeybindCommandList    KeybindID = "command_list"
	KeybindCommandPalette KeybindID = "command_palette"
	KeybindAgentList      KeybindID = "agent_list"
	KeybindSessionList    KeybindID = "session_list"
	KeybindSessionNew     KeybindID = "session_new"
	KeybindSessionRename  KeybindID = "session_rename"
	KeybindSessionDelete  KeybindID = "session_delete"
	KeybindModelList      KeybindID = "model_list"
	KeybindThemeList      KeybindID = "theme_list"
	KeybindSidebarToggle  KeybindID = "sidebar_toggle"
	KeybindPromptPaste    KeybindID = "prompt_paste"
	KeybindHelpToggle     KeybindID = "help_toggle"
	KeybindLogsToggle     KeybindID = "logs_toggle"
	KeybindFilePicker     KeybindID = "file_picker"
	KeybindSessionCompact KeybindID = "session_compact"
	KeybindSessionExport  KeybindID = "session_export"
	KeybindHistoryPrev    KeybindID = "history_previous"
	KeybindHistoryNext    KeybindID = "history_next"
	KeybindDisplayThink   KeybindID = "display_thinking"
	KeybindMessagesCopy   KeybindID = "messages_copy"
	KeybindMessagesUndo   KeybindID = "messages_undo"
	KeybindMessagesRedo   KeybindID = "messages_redo"
)

// defaultLeader is the default value of the <leader> placeholder.
const defaultLeader = "ctrl+x"

// noneValue (and the boolean false) disable a binding in the config.
const noneValue = "none"

// defaultKeybinds are the built-in bindings, matching the pre-Spec-09 keymap so
// that behaviour is unchanged when no keybinds are configured.
var defaultKeybinds = map[KeybindID]string{
	KeybindAppExit:        "ctrl+c",
	KeybindCommandList:    "ctrl+k",
	KeybindCommandPalette: "ctrl+p",
	KeybindAgentList:      "tab",
	KeybindSessionList:    "ctrl+s",
	KeybindSessionNew:     "<leader>n",
	KeybindSessionRename:  "<leader>r",
	KeybindSessionDelete:  "<leader>d",
	KeybindModelList:      "ctrl+o",
	KeybindThemeList:      "ctrl+t",
	KeybindSidebarToggle:  "<leader>b",
	KeybindPromptPaste:    "ctrl+v",
	KeybindHelpToggle:     "ctrl+_",
	KeybindLogsToggle:     "ctrl+l",
	KeybindFilePicker:     "ctrl+f",
	KeybindSessionCompact: "<leader>c",
	KeybindSessionExport:  "<leader>x",
	KeybindHistoryPrev:    "up",
	KeybindHistoryNext:    "down",
	// Like the TS default: /thinking is the discoverable way to toggle it.
	KeybindDisplayThink: noneValue,
	KeybindMessagesCopy: "<leader>y",
	KeybindMessagesUndo: "<leader>u",
	// The TS default for messages_redo is <leader>r, but that suffix is
	// taken by session_rename in this fork; "." keeps the vim redo mnemonic.
	KeybindMessagesRedo: "<leader>.",
}

// Keybinds is the resolved keymap: each id maps to the list of key strings that
// trigger it. A disabled binding has an empty list.
type Keybinds struct {
	Leader   string
	bindings map[KeybindID][]string
}

// Keys returns the key strings bound to id, or nil when it is disabled/unknown.
func (k Keybinds) Keys(id KeybindID) []string {
	return k.bindings[id]
}

// Enabled reports whether id has at least one key.
func (k Keybinds) Enabled(id KeybindID) bool {
	return len(k.bindings[id]) > 0
}

// ResolveKeybinds merges the configured keybinds over the defaults and returns
// the resolved map plus warnings for unknown ids or empty bindings. The second
// return value is never nil.
func ResolveKeybinds(configured map[string]string) (Keybinds, []string) {
	k := Keybinds{bindings: make(map[KeybindID][]string, len(defaultKeybinds))}
	warnings := []string{}

	// Resolve the leader first; it may itself be configured.
	k.Leader = defaultLeader
	if v, ok := configured["leader"]; ok {
		if v == "" || isNone(v) {
			warnings = append(warnings, "keybinds.leader: cannot be disabled")
		} else {
			k.Leader = normalizeKeys(v)[0]
		}
	}

	ids := make([]string, 0, len(defaultKeybinds))
	for id := range defaultKeybinds {
		ids = append(ids, string(id))
	}
	sort.Strings(ids)

	for _, raw := range ids {
		id := KeybindID(raw)
		spec := defaultKeybinds[id]
		if v, ok := configured[raw]; ok {
			spec = v
		}
		keys, err := resolveBinding(spec, k.Leader)
		if err != nil {
			warnings = append(warnings, fmt.Sprintf("keybinds.%s: %v", raw, err))
			keys, _ = resolveBinding(defaultKeybinds[id], k.Leader)
		}
		k.bindings[id] = keys
	}

	// Warn about ids the config sets that we do not know.
	known := map[string]bool{"leader": true}
	for id := range defaultKeybinds {
		known[string(id)] = true
	}
	for raw := range configured {
		if !known[raw] {
			warnings = append(warnings, fmt.Sprintf("keybinds.%s: unknown keybind id, ignored", raw))
		}
	}
	sort.Strings(warnings)
	return k, warnings
}

// resolveBinding expands <leader> and splits a binding into its key list. It
// returns an empty list for "none"/false/empty-with-intent, and an error when a
// token is empty. A leading <leader> becomes a two-keystroke sequence
// ("<leader>d" -> ["ctrl+x", "d"]).
func resolveBinding(spec, leader string) ([]string, error) {
	spec = strings.TrimSpace(spec)
	if spec == "" || isNone(spec) {
		return nil, nil
	}
	if strings.HasPrefix(spec, "<leader>") {
		rest := normalizeKeys(strings.TrimPrefix(spec, "<leader>"))
		keys := append([]string{leader}, rest...)
		if len(keys) == 0 {
			return nil, fmt.Errorf("empty binding")
		}
		return keys, nil
	}
	spec = strings.ReplaceAll(spec, "<leader>", leader)
	keys := normalizeKeys(spec)
	if len(keys) == 0 {
		return nil, fmt.Errorf("empty binding")
	}
	return keys, nil
}

func isNone(spec string) bool {
	s := strings.ToLower(strings.TrimSpace(spec))
	return s == noneValue || s == "false"
}

// normalizeKeys splits a binding on commas and lowercase-trims each token.
func normalizeKeys(spec string) []string {
	parts := strings.Split(spec, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.ToLower(strings.TrimSpace(p)); p != "" {
			out = append(out, p)
		}
	}
	return out
}
