package planner

import (
	"fmt"
	"sort"

	"eed/internal/components"
)

func topoSort(registry *components.Registry, ids []string) ([]string, error) {
	if registry == nil {
		return nil, fmt.Errorf("registry is nil")
	}
	required := make(map[string]bool)
	for _, id := range ids {
		required[id] = true
	}
	indegree := map[string]int{}
	dependents := map[string][]string{}
	for id := range required {
		indegree[id] = 0
	}
	for id := range required {
		manifest, ok := registry.Get(id)
		if !ok {
			continue
		}
		for _, dep := range manifest.Dependencies {
			if !required[dep] {
				continue
			}
			dependents[dep] = append(dependents[dep], id)
			indegree[id]++
		}
	}
	queue := make([]string, 0)
	for _, id := range sortedKeys(indegree) {
		if indegree[id] == 0 {
			queue = append(queue, id)
		}
	}
	ordered := make([]string, 0, len(ids))
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		ordered = append(ordered, current)
		for _, next := range dependents[current] {
			indegree[next]--
			if indegree[next] == 0 {
				queue = append(queue, next)
			}
		}
		sort.Strings(queue)
	}
	if len(ordered) != len(required) {
		return nil, fmt.Errorf("dependency cycle detected")
	}
	return ordered, nil
}

func sortedKeys(m map[string]int) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	return keys
}
