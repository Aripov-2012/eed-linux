package customization

import "strings"

// Selection identifies a component or requested feature from the user's desired state.
type Selection struct {
	ID    string
	Value string
}

// SelectedComponents resolves the normalized desired state into explicit component IDs.
func SelectedComponents(state DesiredState) []string {
	selected := make([]string, 0, 16)
	seen := map[string]bool{}

	appendUnique := func(id string) {
		if id == "" || seen[id] {
			return
		}
		seen[id] = true
		selected = append(selected, id)
	}

	if state.Desktop != "" && state.Desktop != "desktop.none" && state.Desktop != "desktop.existing" {
		appendUnique(state.Desktop)
	}
	for _, purpose := range state.Purposes {
		if purpose == "development" {
			appendUnique("system.portals")
		}
	}
	if state.DisplayMode != "" && state.DisplayMode != "display.none" && state.DisplayMode != "display.existing" {
		appendUnique(state.DisplayMode)
	}
	for tool, enabled := range state.Developer {
		if enabled {
			appendUnique(toolComponentID(tool))
		}
	}
	if state.Editor != "" && state.Editor != "editor.none" {
		appendUnique(state.Editor)
	}
	if state.Theme != "" && state.Theme != "appearance.default" {
		appendUnique(state.Theme)
	}
	for _, id := range state.Additional {
		appendUnique(strings.TrimSpace(id))
	}
	return selected
}

func toolComponentID(tool string) string {
	switch strings.ToLower(strings.TrimSpace(tool)) {
	case "git":
		return "developer.git"
	case "docker":
		return "developer.docker"
	case "python":
		return "developer.python"
	case "nodejs":
		return "developer.nodejs"
	case "go":
		return "developer.go"
	case "rust":
		return "developer.rust"
	default:
		return ""
	}
}
