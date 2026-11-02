package portals

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"eed/internal/portalconfig"
)

type Plan = portalconfig.Plan

func BuildHostPlan(desktop string, installedPackages map[string]bool) (portalconfig.Plan, error) {
	configHome, err := os.UserConfigDir()
	if err != nil {
		return portalconfig.Plan{}, fmt.Errorf("resolve user config directory: %w", err)
	}
	return buildHostPlan(desktop, configHome, installedPackages)
}

func buildHostPlan(desktop, configHome string, installedPackages map[string]bool) (portalconfig.Plan, error) {
	proposal, err := BuildPlan(desktop, configHome, nil, installedPackages)
	if err != nil {
		return portalconfig.Plan{}, err
	}
	configPaths := []string{proposal.ConfigPath}
	genericPath := filepath.Join(configHome, "xdg-desktop-portal", "portals.conf")
	if genericPath != proposal.ConfigPath {
		configPaths = append(configPaths, genericPath)
	}
	var existing []byte
	existingPaths := make([]string, 0, len(configPaths))
	for _, path := range configPaths {
		content, readErr := os.ReadFile(path)
		if readErr != nil {
			if errors.Is(readErr, os.ErrNotExist) {
				continue
			}
			return portalconfig.Plan{}, fmt.Errorf("inspect existing portal configuration %q: %w", path, readErr)
		}
		if content == nil {
			content = []byte{}
		}
		if existing == nil {
			existing = content
		}
		existingPaths = append(existingPaths, path)
	}
	plan, err := BuildPlan(desktop, configHome, existing, installedPackages)
	if err != nil {
		return portalconfig.Plan{}, err
	}
	if len(existingPaths) > 0 {
		plan.ExistingConfigPaths = existingPaths
	}
	return plan, nil
}

func BuildPlan(desktop, configHome string, existing []byte, installedPackages map[string]bool) (portalconfig.Plan, error) {
	desktop = normalizeDesktop(desktop)
	if desktop == "" {
		return portalconfig.Plan{}, fmt.Errorf("desktop environment is required for portal selection")
	}
	if configHome == "" {
		return portalconfig.Plan{}, fmt.Errorf("XDG config home is required for portal configuration planning")
	}

	packages, content, environment := policy(desktop)
	if len(packages) == 0 {
		return portalconfig.Plan{}, fmt.Errorf("no portal policy is defined for desktop %q", desktop)
	}
	filename := "portals.conf"
	if desktop != "default" {
		filename = desktop + "-portals.conf"
	}
	result := portalconfig.Plan{
		Desktop:          desktop,
		ConfigPath:       filepath.Join(configHome, "xdg-desktop-portal", filename),
		Content:          content,
		RequiredPackages: packages,
		Environment:      environment,
		Status:           "configuration_manager_not_implemented",
		Limitations:      []string{"This is a read-only proposal; no configuration file is created or modified."},
	}
	if installedPackages == nil {
		result.PackageInventoryStatus = "unknown"
		result.Limitations = append(result.Limitations, "Package presence could not be determined because the installed-package inventory is unavailable or not applicable on this host.")
	} else {
		result.PackageInventoryStatus = "available"
		for _, pkg := range packages {
			if !installedPackages[pkg] {
				result.MissingPackages = append(result.MissingPackages, pkg)
			}
		}
	}
	if existing != nil {
		result.PreserveExisting = true
		result.UserConfigDetected = true
		result.ExistingConfigPaths = []string{result.ConfigPath}
		result.Status = "existing_user_configuration_preserved"
		result.Limitations = append(result.Limitations, "An existing user portal configuration was detected and will not be overwritten; a configuration backup and ownership record are required before any future deployment.")
	}
	if len(result.MissingPackages) > 0 {
		result.Limitations = append(result.Limitations, "Missing portal packages are reported only; Test Drive will not install them.")
	}
	return result, nil
}

func policy(desktop string) ([]string, string, []string) {
	switch desktop {
	case "hyprland":
		return []string{"xdg-desktop-portal", "xdg-desktop-portal-hyprland", "xdg-desktop-portal-gtk"},
			"[preferred]\ndefault=gtk\norg.freedesktop.impl.portal.FileChooser=gtk\norg.freedesktop.impl.portal.ScreenCast=hyprland\norg.freedesktop.impl.portal.Screenshot=hyprland\n",
			[]string{"XDG_CURRENT_DESKTOP=Hyprland", "XDG_SESSION_TYPE=wayland"}
	case "gnome":
		return []string{"xdg-desktop-portal", "xdg-desktop-portal-gnome"},
			"[preferred]\ndefault=gnome\n",
			[]string{"XDG_CURRENT_DESKTOP=GNOME"}
	case "kde":
		return []string{"xdg-desktop-portal", "xdg-desktop-portal-kde"},
			"[preferred]\ndefault=kde\n",
			[]string{"XDG_CURRENT_DESKTOP=KDE"}
	case "xfce":
		return []string{"xdg-desktop-portal", "xdg-desktop-portal-gtk"},
			"[preferred]\ndefault=gtk\n",
			[]string{"XDG_CURRENT_DESKTOP=XFCE"}
	default:
		return []string{"xdg-desktop-portal", "xdg-desktop-portal-gtk"},
			"[preferred]\ndefault=gtk\n",
			nil
	}
}

func normalizeDesktop(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "desktop.hyprland", "hyprland":
		return "hyprland"
	case "desktop.gnome", "gnome":
		return "gnome"
	case "desktop.kde", "kde", "plasma", "kde plasma":
		return "kde"
	case "desktop.xfce", "xfce":
		return "xfce"
	case "", "desktop.none", "none":
		return "default"
	default:
		return "default"
	}
}
