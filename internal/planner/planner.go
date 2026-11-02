package planner

import (
	"fmt"
	"os"
	"sort"
	"strings"

	"eed/internal/components"
	"eed/internal/config"
	"eed/internal/customization"
	"eed/internal/detection"
)

// BuildPlan turns a normalized user config and detection report into a non-destructive installation plan.
func BuildPlan(cfg config.Config, registry *components.Registry, report detection.Report) (Plan, error) {
	state, err := customization.NormalizeFromConfig(cfg)
	if err != nil {
		return Plan{}, err
	}
	return BuildPlanFromState(state, registry, report)
}

func BuildPlanFromState(state customization.DesiredState, registry *components.Registry, report detection.Report) (Plan, error) {
	if registry == nil {
		return Plan{}, fmt.Errorf("component registry is required")
	}

	selected := customization.SelectedComponents(state)
	resolved, reasons, warnings := resolveRequiredComponents(registry, selected)
	if len(resolved) == 0 && len(selected) == 0 {
		return Plan{
			SchemaVersion: 1,
			Profile:       state.Profile,
			TargetOS:      "arch",
			HostOS:        normalizeHostOS(report),
			Installable:   false,
			Components:    nil,
			Packages:      nil,
			Warnings:      []string{"No explicit components selected."},
			Status:        "ready",
		}, nil
	}

	orderedIDs, err := topoSort(registry, resolved)
	if err != nil {
		warnings = append(warnings, err.Error())
	}
	if len(orderedIDs) == 0 {
		orderedIDs = append([]string{}, resolved...)
	}

	conflicts := detectConflicts(registry, resolved)
	planWarnings := make([]string, 0, len(warnings))
	planErrors := make([]string, 0)
	for _, warning := range warnings {
		if strings.Contains(warning, "missing component") || strings.Contains(warning, "not found in registry") {
			planErrors = append(planErrors, warning)
			continue
		}
		planWarnings = append(planWarnings, warning)
	}
	if len(conflicts) > 0 {
		planErrors = append(planErrors, conflicts...)
	}
	hostOS := normalizeHostOS(report)
	targetOS := "arch"
	installable := hostOS == "arch" || strings.EqualFold(hostOS, "arch linux")
	if !installable {
		planWarnings = append(planWarnings, "Current operating system is not Arch Linux; the plan is diagnostic only and not installable on this host.")
	}
	if len(planErrors) > 0 {
		installable = false
	}

	componentsPlan := make([]ComponentPlan, 0, len(orderedIDs))
	for _, id := range orderedIDs {
		manifest, ok := registry.Get(id)
		if !ok {
			componentsPlan = append(componentsPlan, ComponentPlan{
				ID:       id,
				Name:     id,
				Selected: true,
				Reason:   reasons[id],
				Status:   "blocked",
			})
			continue
		}
		componentStatus := "planned"
		if detectedAsInstalled(report, id) {
			componentStatus = "already_present"
		}
		if isUnsupportedComponent(registry, id) {
			componentStatus = "unsupported"
		}
		if len(planErrors) > 0 {
			componentStatus = "blocked"
		}
		componentsPlan = append(componentsPlan, ComponentPlan{
			ID:           id,
			Name:         manifest.Name,
			Selected:     true,
			Reason:       reasons[id],
			Status:       componentStatus,
			Dependencies: manifest.Dependencies,
		})
	}
	if len(componentsPlan) == 0 {
		componentsPlan = []ComponentPlan{}
	}

	packageList := collectPackages(registry, resolved, state.Desktop)
	status := "ready"
	if len(planErrors) > 0 {
		status = "blocked"
	} else if len(planWarnings) > 0 {
		status = "ready_with_warnings"
	}
	if !installable {
		status = "blocked"
	}

	return Plan{
		SchemaVersion: 1,
		Profile:       state.Profile,
		TargetOS:      targetOS,
		HostOS:        hostOS,
		Installable:   installable,
		Components:    componentsPlan,
		Packages:      packageList,
		Warnings:      planWarnings,
		Errors:        planErrors,
		Status:        status,
	}, nil
}

func normalizeHostOS(report detection.Report) string {
	if report.System.OS.ID != "" {
		return strings.ToLower(report.System.OS.ID)
	}
	if report.System.OS.Name != "" {
		return strings.ToLower(report.System.OS.Name)
	}
	return "unknown"
}

func isUnsupportedComponent(registry *components.Registry, id string) bool {
	manifest, ok := registry.Get(id)
	if !ok {
		return true
	}
	if len(manifest.SupportedOS) == 0 {
		return true
	}
	return false
}

func contains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func detectedAsInstalled(report detection.Report, id string) bool {
	switch id {
	case "desktop.hyprland":
		return strings.Contains(strings.ToLower(report.System.Desktop), "hyprland")
	case "desktop.kde":
		return strings.Contains(strings.ToLower(report.System.Desktop), "kde") || strings.Contains(strings.ToLower(report.System.Desktop), "plasma")
	case "desktop.gnome":
		return strings.Contains(strings.ToLower(report.System.Desktop), "gnome")
	case "desktop.xfce":
		return strings.Contains(strings.ToLower(report.System.Desktop), "xfce")
	case "editor.vscode":
		for _, cmd := range report.Software.Commands {
			if strings.EqualFold(cmd.Name, "code") || strings.EqualFold(cmd.Name, "code-insiders") {
				return cmd.Installed
			}
		}
	case "editor.neovim":
		for _, cmd := range report.Software.Commands {
			if strings.EqualFold(cmd.Name, "nvim") {
				return cmd.Installed
			}
		}
	case "developer.git":
		for _, cmd := range report.Software.Commands {
			if strings.EqualFold(cmd.Name, "git") {
				return cmd.Installed
			}
		}
	}
	return false
}

func defaultManifestDir() string {
	candidates := []string{"components/manifests", "components", "profiles"}
	for _, dir := range candidates {
		if _, err := os.Stat(dir); err == nil {
			return dir
		}
	}
	return "components/manifests"
}

func BuildPlanFromPaths(cfg config.Config, manifestDir string, report detection.Report) (Plan, error) {
	if manifestDir == "" {
		manifestDir = defaultManifestDir()
	}
	registry, err := components.LoadFromDir(manifestDir)
	if err != nil {
		return Plan{}, err
	}
	return BuildPlan(cfg, registry, report)
}

func ensureSortedStrings(values []string) []string {
	out := append([]string(nil), values...)
	sort.Strings(out)
	return out
}
