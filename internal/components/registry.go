package components

import (
	"fmt"
	"sort"
)

// Registry stores known component manifests keyed by component ID.
type Registry struct {
	Components map[string]Manifest
	Order      []string
}

func NewRegistry() *Registry {
	return &Registry{Components: map[string]Manifest{}}
}

func (r *Registry) Add(m Manifest) error {
	m.ID = normalizeManifestID(m.ID)
	if m.ID == "" {
		return fmt.Errorf("manifest is missing id")
	}
	if _, exists := r.Components[m.ID]; exists {
		return fmt.Errorf("duplicate component id %q", m.ID)
	}
	if err := validateManifest(m); err != nil {
		return fmt.Errorf("component %q: %w", m.ID, err)
	}
	r.Components[m.ID] = m
	r.Order = append(r.Order, m.ID)
	sort.Strings(r.Order)
	return nil
}

func (r *Registry) Has(id string) bool {
	_, ok := r.Components[id]
	return ok
}

func (r *Registry) Get(id string) (Manifest, bool) {
	m, ok := r.Components[id]
	return m, ok
}

func (r *Registry) ValidateDependencies() error {
	for id, manifest := range r.Components {
		for _, dep := range manifest.Dependencies {
			if _, ok := r.Components[dep]; !ok {
				return fmt.Errorf("component %q references missing dependency %q", id, dep)
			}
		}
	}
	return nil
}

func (r *Registry) DetectCycles() error {
	visited := map[string]bool{}
	stack := map[string]bool{}
	var walk func(string) error
	walk = func(id string) error {
		if stack[id] {
			return fmt.Errorf("dependency cycle detected at %q", id)
		}
		if visited[id] {
			return nil
		}
		visited[id] = true
		stack[id] = true
		for _, dep := range r.Components[id].Dependencies {
			if err := walk(dep); err != nil {
				return err
			}
		}
		delete(stack, id)
		return nil
	}
	for id := range r.Components {
		if err := walk(id); err != nil {
			return err
		}
	}
	return nil
}
