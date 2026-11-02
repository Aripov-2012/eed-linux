package config

import (
	"fmt"
	"strings"
)

var allowedUsagePurposes = map[string]bool{
	"development": true,
	"everyday":    true,
	"gaming":      true,
	"minimal":     true,
	"multimedia":  true,
	"other":       true,
}

var allowedDesktopEnvironments = map[string]bool{
	"gnome":               true,
	"kde":                 true,
	"xfce":                true,
	"hyprland":            true,
	"sway":                true,
	"i3":                  true,
	"existing":            true,
	"none":                true,
	"kde plasma":          true,
	"keep existing":       true,
	"no graphical":        true,
	"no graphical system": true,
}

var allowedDisplayManagers = map[string]bool{
	"none":     true,
	"gdm":      true,
	"sddm":     true,
	"lightdm":  true,
	"existing": true,
}

var allowedEditors = map[string]bool{
	"none":   true,
	"neovim": true,
	"vscode": true,
	"vim":    true,
	"helix":  true,
	"emacs":  true,
}

var allowedThemes = map[string]bool{
	"catppuccin-mocha": true,
	"tokyo-night":      true,
	"default":          true,
	"catppuccin":       true,
	"tokyo-night-dark": true,
}

func Validate(cfg Config) error {
	if cfg.SchemaVersion != 1 {
		return fmt.Errorf("schema_version must be 1")
	}

	if strings.TrimSpace(cfg.Profile.Name) == "" {
		return fmt.Errorf("profile.name must not be empty")
	}

	for _, purpose := range cfg.Profile.Purposes {
		if !allowedUsagePurposes[strings.ToLower(purpose)] {
			return fmt.Errorf("unsupported profile purpose %q", purpose)
		}
	}

	if !allowedDesktopEnvironments[strings.ToLower(cfg.Desktop.Environment)] {
		return fmt.Errorf("unsupported desktop environment %q", cfg.Desktop.Environment)
	}

	if !allowedDisplayManagers[strings.ToLower(cfg.Desktop.DisplayManager)] {
		return fmt.Errorf("unsupported display manager %q", cfg.Desktop.DisplayManager)
	}

	if !allowedEditors[strings.ToLower(cfg.Editor.Name)] {
		return fmt.Errorf("unsupported editor %q", cfg.Editor.Name)
	}

	if !allowedThemes[strings.ToLower(cfg.Appearance.Theme)] {
		return fmt.Errorf("unsupported appearance theme %q", cfg.Appearance.Theme)
	}

	return nil
}
