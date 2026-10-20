package agentdef

import (
	"fmt"
	"sync"
)

// DefaultName is the fallback agent used when none is configured.
const DefaultName = "coder"

// Registry holds agent definitions keyed by name. It is safe for concurrent use.
type Registry struct {
	mu      sync.RWMutex
	defs    map[string]*Definition
	order   []string
	defName string
}

// NewRegistry returns an empty registry.
func NewRegistry() *Registry {
	return &Registry{defs: make(map[string]*Definition)}
}

// Register adds or replaces a definition. Names must be non-empty.
func (r *Registry) Register(d *Definition) error {
	if d == nil || d.Name == "" {
		return fmt.Errorf("agent definition requires a name")
	}
	if d.Mode == "" {
		d.Mode = ModeAll
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, ok := r.defs[d.Name]; !ok {
		r.order = append(r.order, d.Name)
	}
	r.defs[d.Name] = d
	return nil
}

// Get returns the definition with the given name.
func (r *Registry) Get(name string) (*Definition, bool) {
	r.mu.RLock()
	defer r.mu.RUnlock()
	d, ok := r.defs[name]
	return d, ok
}

// List returns all definitions in registration order.
func (r *Registry) List() []*Definition {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]*Definition, 0, len(r.order))
	for _, name := range r.order {
		if d, ok := r.defs[name]; ok {
			out = append(out, d)
		}
	}
	return out
}

// ListByMode returns enabled definitions usable in the given mode. Hidden agents
// are excluded unless includeHidden is true. A definition with ModeAll matches
// any mode.
func (r *Registry) ListByMode(m Mode, includeHidden bool) []*Definition {
	out := make([]*Definition, 0)
	for _, d := range r.List() {
		if d.Disabled {
			continue
		}
		if !includeHidden && d.Hidden {
			continue
		}
		if m == ModeAll || d.Mode == ModeAll || d.Mode == m {
			out = append(out, d)
		}
	}
	return out
}

// Default returns the configured default agent, falling back to "coder" and
// then to the first enabled definition.
func (r *Registry) Default() string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	if r.defName != "" {
		if d, ok := r.defs[r.defName]; ok && !d.Disabled {
			return r.defName
		}
	}
	if d, ok := r.defs[DefaultName]; ok && !d.Disabled {
		return DefaultName
	}
	for _, name := range r.order {
		if d, ok := r.defs[name]; ok && !d.Disabled {
			return name
		}
	}
	return ""
}

// SetDefault sets the preferred default agent name.
func (r *Registry) SetDefault(name string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	r.defName = name
}

var (
	globalMu sync.RWMutex
	global   *Registry
)

// Get returns the process-wide registry, creating an empty one if unset.
func Get() *Registry {
	globalMu.RLock()
	r := global
	globalMu.RUnlock()
	if r != nil {
		return r
	}
	globalMu.Lock()
	defer globalMu.Unlock()
	if global == nil {
		global = NewRegistry()
	}
	return global
}

// Set replaces the process-wide registry.
func Set(r *Registry) {
	globalMu.Lock()
	defer globalMu.Unlock()
	global = r
}
