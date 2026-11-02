package planner

import (
	"sort"
	"strings"

	"eed/internal/components"
)

func resolveRequiredComponents(registry *components.Registry, selected []string) ([]string, map[string]string, []string) {
	required := make([]string, 0, len(selected))
	seen := map[string]bool{}
	reasonByID := map[string]string{}
	warnings := make([]string, 0)
	queue := append([]string{}, selected...)
	for _, id := range selected {
		reasonByID[id] = "user_selection"
	}
	for len(queue) > 0 {
		current := queue[0]
		queue = queue[1:]
		if current == "" {
			continue
		}
		if seen[current] {
			continue
		}
		seen[current] = true
		required = append(required, current)
		manifest, ok := registry.Get(current)
		if !ok {
			if strings.HasPrefix(current, "display.") {
				continue
			}
			warnings = append(warnings, "component manifest not found in registry: "+current)
			continue
		}
		for _, dep := range manifest.Dependencies {
			if dep == "" {
				continue
			}
			if _, exists := registry.Get(dep); !exists {
				warnings = append(warnings, "component "+current+" depends on missing component manifest: "+dep)
				continue
			}
			if !seen[dep] {
				reasonByID[dep] = "dependency"
				queue = append(queue, dep)
			}
		}
	}
	sort.Strings(required)
	return required, reasonByID, warnings
}

func collectPackages(registry *components.Registry, ids []string, desktop string) []string {
	seen := map[string]bool{}
	pkgs := make([]string, 0)
	for _, id := range ids {
		manifest, ok := registry.Get(id)
		if !ok {
			continue
		}
		packages := manifest.Packages
		if len(manifest.PackageVariants) > 0 {
			if variant, ok := manifest.PackageVariants[desktop]; ok {
				packages = variant
			} else if variant, ok := manifest.PackageVariants["default"]; ok {
				packages = variant
			} else {
				packages = nil
			}
		}
		for _, pkg := range packages {
			pkg = strings.TrimSpace(pkg)
			if pkg == "" || seen[pkg] {
				continue
			}
			seen[pkg] = true
			pkgs = append(pkgs, pkg)
		}
	}
	sort.Strings(pkgs)
	return pkgs
}
