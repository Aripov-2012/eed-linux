package checks

import (
	"context"
	"fmt"
	"strings"

	"eed/internal/verification"
)

type desktopBinary struct {
	component string
	binary    string
}

var desktops = []desktopBinary{
	{component: "desktop.gnome", binary: "gnome-shell"},
	{component: "desktop.kde", binary: "plasmashell"},
	{component: "desktop.xfce", binary: "xfce4-session"},
	{component: "desktop.hyprland", binary: "Hyprland"},
	{component: "desktop.sway", binary: "sway"},
	{component: "desktop.i3", binary: "i3"},
}

func registerDesktopChecks(registry *verification.Registry) error {
	for _, desktop := range desktops {
		entry := desktop
		id := strings.ReplaceAll(entry.component, ".", "_") + ".binary"
		if err := registry.Register(verification.Check{
			ID:          id,
			ComponentID: entry.component,
			Name:        fmt.Sprintf("%s executable", entry.binary),
			Description: "Verify the selected desktop environment executable is available.",
			Severity:    "required",
			Required:    true,
			Run: func(_ context.Context, input verification.Context) verification.CheckResult {
				path, err := input.Runner.LookPath(entry.binary)
				if err != nil {
					return verification.CheckResult{Status: verification.StatusFail, Message: fmt.Sprintf("required desktop executable %q is not available", entry.binary)}
				}
				return verification.CheckResult{Status: verification.StatusPass, Message: path}
			},
		}); err != nil {
			return err
		}
	}
	return nil
}
