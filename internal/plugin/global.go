package plugin

import "sync"

var (
	globalMu      sync.RWMutex
	globalManager *Manager
)

// SetManager installs the process-wide plugin manager.
func SetManager(m *Manager) {
	globalMu.Lock()
	globalManager = m
	globalMu.Unlock()
}

// GetManager returns the process-wide plugin manager, or nil when none is set.
func GetManager() *Manager {
	globalMu.RLock()
	defer globalMu.RUnlock()
	return globalManager
}
