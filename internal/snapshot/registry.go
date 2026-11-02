package snapshot

import "sync"

var (
	registryMu sync.RWMutex
	registry   = map[string]Backend{}
)

// Register adds a backend to the global registry.
func Register(backend Backend) {
	if backend == nil {
		return
	}
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[backend.Name()] = backend
}

// Lookup looks up a backend by name.
func Lookup(name string) (Backend, bool) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	backend, ok := registry[name]
	return backend, ok
}

// Registered returns all stored backends.
func Registered() []Backend {
	registryMu.RLock()
	defer registryMu.RUnlock()
	out := make([]Backend, 0, len(registry))
	for _, backend := range registry {
		out = append(out, backend)
	}
	return out
}
