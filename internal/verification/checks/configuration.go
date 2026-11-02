package checks

import (
	"context"
	"os"
	"path/filepath"

	"eed/internal/verification"
)

func registerConfigurationChecks(registry *verification.Registry) error {
	return registry.Register(verification.Check{
		ID:          "desktop_hyprland.configuration",
		ComponentID: "desktop.hyprland",
		Name:        "Hyprland configuration",
		Description: "Check for the conventional user Hyprland configuration file without modifying it.",
		Severity:    "optional",
		Required:    false,
		Run: func(_ context.Context, _ verification.Context) verification.CheckResult {
			home, err := os.UserHomeDir()
			if err != nil {
				return verification.CheckResult{Status: verification.StatusUnknown, Message: "unable to resolve current user's home directory"}
			}
			path := filepath.Join(home, ".config", "hypr", "hyprland.conf")
			info, err := os.Stat(path)
			if err != nil {
				return verification.CheckResult{Status: verification.StatusWarning, Message: "Hyprland configuration was not found at " + path}
			}
			if !info.Mode().IsRegular() {
				return verification.CheckResult{Status: verification.StatusFail, Message: "Hyprland configuration path is not a regular file"}
			}
			return verification.CheckResult{Status: verification.StatusPass, Message: "configuration file exists; syntax validation is unavailable in this milestone"}
		},
	})
}
