package verification

import (
	"fmt"
	"sort"
	"sync"
)

type Check struct {
	ID          string
	ComponentID string
	Name        string
	Description string
	Severity    string
	Required    bool
	Run         CheckFunc
}

type Registry struct {
	mu     sync.RWMutex
	checks map[string]Check
}

func NewRegistry() *Registry {
	return &Registry{checks: make(map[string]Check)}
}

func (r *Registry) Register(check Check) error {
	if check.ID == "" || check.ComponentID == "" || check.Name == "" || check.Run == nil {
		return fmt.Errorf("verification check requires id, component id, name, and run function")
	}
	r.mu.Lock()
	defer r.mu.Unlock()
	if _, exists := r.checks[check.ID]; exists {
		return fmt.Errorf("verification check %q is already registered", check.ID)
	}
	r.checks[check.ID] = check
	return nil
}

func (r *Registry) Checks() []Check {
	r.mu.RLock()
	defer r.mu.RUnlock()
	checks := make([]Check, 0, len(r.checks))
	for _, check := range r.checks {
		checks = append(checks, check)
	}
	sort.Slice(checks, func(i, j int) bool { return checks[i].ID < checks[j].ID })
	return checks
}
