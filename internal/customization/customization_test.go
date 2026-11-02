package customization

import (
	"testing"

	"eed/internal/config"
)

func TestNormalizeFromConfig(t *testing.T) {
	cfg := config.Config{
		SchemaVersion: 1,
		Profile:       config.ProfileConfig{Name: "developer", Purposes: []string{"development"}},
		Desktop:       config.DesktopConfig{Environment: "hyprland", DisplayManager: "sddm"},
		Developer:     config.DeveloperConfig{Git: true, Docker: true, Python: true},
		Editor:        config.EditorConfig{Name: "neovim"},
		Appearance:    config.AppearanceConfig{Theme: "catppuccin-mocha"},
	}

	state, err := NormalizeFromConfig(cfg)
	if err != nil {
		t.Fatalf("normalize config: %v", err)
	}
	if state.Desktop != "desktop.hyprland" {
		t.Fatalf("unexpected desktop: %s", state.Desktop)
	}
	if !state.Developer["git"] || !state.Developer["docker"] || !state.Developer["python"] {
		t.Fatal("expected developer tools to be enabled")
	}
}

func TestSelectedComponentsIncludesDesktopAndEditor(t *testing.T) {
	state := DesiredState{
		Profile:     "developer",
		Purposes:    []string{"development"},
		Desktop:     "desktop.kde",
		DisplayMode: "display.sddm",
		Developer:   map[string]bool{"git": true},
		Editor:      "editor.vscode",
		Theme:       "appearance.catppuccin",
	}
	selected := SelectedComponents(state)
	if len(selected) < 5 {
		t.Fatalf("expected several selections, got %v", selected)
	}
	if !contains(selected, "desktop.kde") {
		t.Fatal("desktop selection missing")
	}
	if !contains(selected, "editor.vscode") {
		t.Fatal("editor selection missing")
	}
}

func contains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}
