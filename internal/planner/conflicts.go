package planner

import (
	"eed/internal/components"
	"sort"
)

func detectConflicts(registry *components.Registry, ids []string) []string {
	if registry == nil {
		return nil
	}
	seen := map[string]bool{}
	for _, id := range ids {
		seen[id] = true
	}
	conflicts := make([]string, 0)
	for _, id := range ids {
		manifest, ok := registry.Get(id)
		if !ok {
			continue
		}
		for _, conflict := range manifest.Conflicts {
			if seen[conflict] {
				conflicts = append(conflicts, id+" conflicts with "+conflict)
			}
		}
	}
	sort.Strings(conflicts)
	return conflicts
}
