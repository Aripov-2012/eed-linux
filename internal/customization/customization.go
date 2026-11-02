package customization

import (
	"fmt"
	"strings"

	"eed/internal/config"
)

// DesiredState is the normalized desired result after user preferences are applied.
type DesiredState struct {
	Profile     string          `json:"profile"`
	Purposes    []string        `json:"purposes,omitempty"`
	Desktop     string          `json:"desktop,omitempty"`
	DisplayMode string          `json:"display_mode,omitempty"`
	Developer   map[string]bool `json:"developer,omitempty"`
	Editor      string          `json:"editor,omitempty"`
	Theme       string          `json:"theme,omitempty"`
	Additional  []string        `json:"additional,omitempty"`
	Warnings    []string        `json:"warnings,omitempty"`
	Errors      []string        `json:"errors,omitempty"`
}

// NormalizeFromConfig transforms the existing TOML configuration into a desired-state representation.
func NormalizeFromConfig(cfg config.Config) (DesiredState, error) {
	state := DesiredState{
		Profile:     strings.TrimSpace(cfg.Profile.Name),
		Purposes:    normalizePurposes(cfg.Profile.Purposes),
		Desktop:     normalizeDesktop(cfg.Desktop.Environment),
		DisplayMode: normalizeDisplay(cfg.Desktop.DisplayManager),
		Developer: map[string]bool{
			"git":    cfg.Developer.Git,
			"docker": cfg.Developer.Docker,
			"python": cfg.Developer.Python,
			"nodejs": cfg.Developer.NodeJS,
			"go":     cfg.Developer.Go,
			"rust":   cfg.Developer.Rust,
		},
		Editor:     normalizeEditor(cfg.Editor.Name),
		Theme:      normalizeTheme(cfg.Appearance.Theme),
		Additional: []string{},
	}

	if state.Profile == "" {
		state.Profile = "custom"
	}
	if err := Validate(state); err != nil {
		return DesiredState{}, err
	}
	return state, nil
}

func normalizePurposes(values []string) []string {
	if len(values) == 0 {
		return []string{"other"}
	}
	out := make([]string, 0, len(values))
	seen := map[string]bool{}
	for _, value := range values {
		v := normalizePurpose(value)
		if v == "" || seen[v] {
			continue
		}
		seen[v] = true
		out = append(out, v)
	}
	if len(out) == 0 {
		return []string{"other"}
	}
	return out
}

func normalizePurpose(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "development", "dev":
		return "development"
	case "everyday", "daily", "general":
		return "everyday"
	case "gaming", "game":
		return "gaming"
	case "minimal", "lite":
		return "minimal"
	case "multimedia", "media":
		return "multimedia"
	default:
		return "other"
	}
}

func normalizeDesktop(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "gnome":
		return "desktop.gnome"
	case "kde", "kde plasma":
		return "desktop.kde"
	case "xfce":
		return "desktop.xfce"
	case "hyprland":
		return "desktop.hyprland"
	case "sway":
		return "desktop.sway"
	case "i3":
		return "desktop.i3"
	case "existing":
		return "desktop.existing"
	case "none", "no graphical", "no graphical environment", "no-graphics":
		return "desktop.none"
	default:
		return "desktop.none"
	}
}

func normalizeDisplay(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "gdm":
		return "display.gdm"
	case "sddm":
		return "display.sddm"
	case "lightdm":
		return "display.lightdm"
	case "existing":
		return "display.existing"
	case "none":
		return "display.none"
	default:
		return "display.none"
	}
}

func normalizeEditor(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "neovim", "nvim":
		return "editor.neovim"
	case "vscode", "code":
		return "editor.vscode"
	default:
		return "editor.none"
	}
}

func normalizeTheme(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "catppuccin-mocha", "catppuccin":
		return "appearance.catppuccin"
	case "tokyo-night", "tokyo night":
		return "appearance.tokyonight"
	case "default", "other":
		return "appearance.default"
	default:
		return "appearance.default"
	}
}

func Validate(state DesiredState) error {
	if strings.TrimSpace(state.Profile) == "" {
		return fmt.Errorf("profile name is required")
	}
	for _, purpose := range state.Purposes {
		if !isValidPurpose(purpose) {
			return fmt.Errorf("unsupported purpose: %s", purpose)
		}
	}
	if !isValidDesktop(state.Desktop) {
		return fmt.Errorf("unsupported desktop selection: %s", state.Desktop)
	}
	if !isValidDisplay(state.DisplayMode) {
		return fmt.Errorf("unsupported display manager selection: %s", state.DisplayMode)
	}
	if !isValidEditor(state.Editor) {
		return fmt.Errorf("unsupported editor selection: %s", state.Editor)
	}
	if !isValidTheme(state.Theme) {
		return fmt.Errorf("unsupported theme selection: %s", state.Theme)
	}
	return nil
}

func isValidPurpose(v string) bool {
	switch strings.ToLower(v) {
	case "development", "everyday", "gaming", "minimal", "multimedia", "other":
		return true
	default:
		return false
	}
}

func isValidDesktop(v string) bool {
	switch strings.ToLower(v) {
	case "desktop.none", "desktop.gnome", "desktop.kde", "desktop.xfce", "desktop.hyprland", "desktop.sway", "desktop.i3", "desktop.existing":
		return true
	default:
		return false
	}
}

func isValidDisplay(v string) bool {
	switch strings.ToLower(v) {
	case "display.none", "display.gdm", "display.sddm", "display.lightdm", "display.existing":
		return true
	default:
		return false
	}
}

func isValidEditor(v string) bool {
	switch strings.ToLower(v) {
	case "editor.none", "editor.neovim", "editor.vscode":
		return true
	default:
		return false
	}
}

func isValidTheme(v string) bool {
	switch strings.ToLower(v) {
	case "appearance.default", "appearance.catppuccin", "appearance.tokyonight":
		return true
	default:
		return false
	}
}
