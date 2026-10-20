package util

import (
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	zone "github.com/lrstanley/bubblezone"
)

var zoneGlobalOnce sync.Once

// ensureZoneGlobal initializes the bubblezone global manager once. The TUI does
// this at boot; tests reach it through the zone set.
func ensureZoneGlobal() {
	zoneGlobalOnce.Do(func() { zone.NewGlobal() })
}

// ZoneSet tracks the bubblezone ids rendered by a component so a mouse event can
// be resolved back to the element under the cursor (Spec 09). bubblezone only
// resolves positions given an id, so the set remembers which ids to test.
type ZoneSet struct {
	ids []string
}

// NewZoneSet creates an empty zone set.
func NewZoneSet() *ZoneSet { return &ZoneSet{} }

// Mark wraps content in a bubblezone marker under id and remembers the id.
func (z *ZoneSet) Mark(id, content string) string {
	if z == nil {
		return content
	}
	ensureZoneGlobal()
	for _, existing := range z.ids {
		if existing == id {
			return zone.Mark(id, content)
		}
	}
	z.ids = append(z.ids, id)
	return zone.Mark(id, content)
}

// At returns the id of the top-most zone containing the mouse position, testing
// most-recently-added ids first.
func (z *ZoneSet) At(msg tea.MouseMsg) (string, bool) {
	if z == nil {
		return "", false
	}
	ensureZoneGlobal()
	for i := len(z.ids) - 1; i >= 0; i-- {
		if info := zone.Get(z.ids[i]); info != nil && info.InBounds(msg) {
			return z.ids[i], true
		}
	}
	return "", false
}

// Reset forgets every id; call it at the start of a render pass.
func (z *ZoneSet) Reset() {
	if z == nil {
		return
	}
	z.ids = z.ids[:0]
}
