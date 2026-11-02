package portals

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDesktopSpecificPortalPolicies(t *testing.T) {
	tests := []struct {
		desktop        string
		defaultBackend string
		packages       []string
	}{
		{"hyprland", "gtk", []string{"xdg-desktop-portal", "xdg-desktop-portal-hyprland", "xdg-desktop-portal-gtk"}},
		{"gnome", "gnome", []string{"xdg-desktop-portal", "xdg-desktop-portal-gnome"}},
		{"kde", "kde", []string{"xdg-desktop-portal", "xdg-desktop-portal-kde"}},
		{"xfce", "gtk", []string{"xdg-desktop-portal", "xdg-desktop-portal-gtk"}},
		{"none", "gtk", []string{"xdg-desktop-portal", "xdg-desktop-portal-gtk"}},
	}
	for _, test := range tests {
		t.Run(test.desktop, func(t *testing.T) {
			plan, err := BuildPlan(test.desktop, "/home/test/.config", nil, map[string]bool{})
			if err != nil {
				t.Fatal(err)
			}
			if !strings.Contains(plan.Content, "default="+test.defaultBackend) {
				t.Fatalf("unexpected backend selection: %s", plan.Content)
			}
			if len(plan.RequiredPackages) != len(test.packages) {
				t.Fatalf("unexpected package set for %s: %v", test.desktop, plan.RequiredPackages)
			}
			for index, pkg := range test.packages {
				if plan.RequiredPackages[index] != pkg {
					t.Fatalf("unexpected package set for %s: %v", test.desktop, plan.RequiredPackages)
				}
			}
			if test.desktop == "hyprland" &&
				(!strings.Contains(plan.Content, "FileChooser=gtk") ||
					!strings.Contains(plan.Content, "ScreenCast=hyprland") ||
					!strings.Contains(plan.Content, "Screenshot=hyprland")) {
				t.Fatalf("Hyprland should use GTK file chooser and Hyprland capture portals: %s", plan.Content)
			}
			if strings.Contains(plan.Content, "hyprland") && test.desktop != "hyprland" {
				t.Fatalf("Hyprland portal must not be forced on %s: %s", test.desktop, plan.Content)
			}
		})
	}
}

func TestPortalPlanPreservesExistingUserConfiguration(t *testing.T) {
	existing := []byte("[preferred]\ndefault=custom\n")
	plan, err := BuildPlan("hyprland", "/home/test/.config", existing, map[string]bool{
		"xdg-desktop-portal": true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.PreserveExisting || !plan.UserConfigDetected ||
		plan.Status != "existing_user_configuration_preserved" {
		t.Fatalf("existing user portal config must never be replaced: %+v", plan)
	}
	if !strings.Contains(string(existing), "default=custom") ||
		!strings.Contains(plan.Content, "default=gtk") {
		t.Fatalf("expected existing config and proposed config to remain distinct: %+v", plan)
	}
	if len(plan.MissingPackages) != 2 || plan.MissingPackages[0] != "xdg-desktop-portal-hyprland" ||
		plan.MissingPackages[1] != "xdg-desktop-portal-gtk" {
		t.Fatalf("optional portal package gaps should be explicit: %+v", plan.MissingPackages)
	}
}

func TestPortalPlanPreservesEmptyAndGenericUserConfigurationWithoutWriting(t *testing.T) {
	emptyPlan, err := BuildPlan("gnome", "/home/test/.config", []byte{}, map[string]bool{})
	if err != nil {
		t.Fatal(err)
	}
	if !emptyPlan.PreserveExisting || !emptyPlan.UserConfigDetected {
		t.Fatalf("an empty but existing config must still be preserved: %+v", emptyPlan)
	}

	configHome := t.TempDir()
	configDir := filepath.Join(configHome, "xdg-desktop-portal")
	if err := os.MkdirAll(configDir, 0o700); err != nil {
		t.Fatal(err)
	}
	genericPath := filepath.Join(configDir, "portals.conf")
	if err := os.WriteFile(genericPath, []byte{}, 0o600); err != nil {
		t.Fatal(err)
	}
	plan, err := buildHostPlan("hyprland", configHome, map[string]bool{})
	if err != nil {
		t.Fatal(err)
	}
	if !plan.PreserveExisting || len(plan.ExistingConfigPaths) != 1 || plan.ExistingConfigPaths[0] != genericPath {
		t.Fatalf("generic user configuration must be detected and preserved: %+v", plan)
	}
	if _, err := os.Stat(plan.ConfigPath); !os.IsNotExist(err) {
		t.Fatalf("planning must not create desktop-specific configuration %q: err=%v", plan.ConfigPath, err)
	}
}

func TestPortalPlanDoesNotCallUnknownInventoryPackagesMissing(t *testing.T) {
	plan, err := BuildPlan("kde", "/home/test/.config", nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if plan.PackageInventoryStatus != "unknown" || len(plan.MissingPackages) != 0 {
		t.Fatalf("unavailable package inventory must not be reported as missing packages: %+v", plan)
	}
}
