package compatibility

import (
	"eed/internal/components"
)

func desktopCompatibilityChecks(registry *components.Registry, selected []string) []Check {
	checks := make([]Check, 0)
	if registry == nil {
		return checks
	}
	seen := map[string]bool{}
	for _, id := range selected {
		if seen[id] {
			continue
		}
		manifest, ok := registry.Get(id)
		if !ok {
			continue
		}
		seen[id] = true
		for _, conflict := range manifest.Conflicts {
			if seen[conflict] {
				checks = append(checks, Check{Name: id, Status: "incompatible", Message: "Component " + id + " conflicts with selected component " + conflict})
			}
		}
	}
	return checks
}
