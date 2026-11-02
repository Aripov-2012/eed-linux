package planner

import (
	"testing"

	"eed/internal/components"
	"eed/internal/config"
	"eed/internal/customization"
	"eed/internal/detection"
)

func TestBuildPlanAddsDependenciesAndDeduplicatesPackages(t *testing.T) {
	cfg := config.Config{
		Profile:    config.ProfileConfig{Name: "developer", Purposes: []string{"development"}},
		Desktop:    config.DesktopConfig{Environment: "hyprland", DisplayManager: "sddm"},
		Developer:  config.DeveloperConfig{Git: true},
		Editor:     config.EditorConfig{Name: "vscode"},
		Appearance: config.AppearanceConfig{Theme: "catppuccin-mocha"},
	}
	report := detection.Report{
		System: detection.SystemReport{
			OS:      detection.OSDetails{ID: "arch", Name: "Arch Linux"},
			Desktop: "Hyprland",
		},
		Software: detection.SoftwareReport{Commands: []detection.CommandInfo{{Name: "git", Installed: true}, {Name: "code", Installed: true}}},
	}
	registry, err := components.LoadFromDir("../../components/manifests")
	if err != nil {
		t.Fatalf("load registry: %v", err)
	}
	plan, err := BuildPlan(cfg, registry, report)
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}
	if plan.Status != "ready" {
		t.Fatalf("expected ready plan, got %q", plan.Status)
	}
	if !sliceContains(plan.Packages, "git") || !sliceContains(plan.Packages, "hyprland") {
		t.Fatalf("expected packages to include required dependencies, got %#v", plan.Packages)
	}
	if len(plan.Packages) != len(uniqueStrings(plan.Packages)) {
		t.Fatalf("expected packages to be deduplicated, got %#v", plan.Packages)
	}
}

func TestBuildPlanRejectsConflictingSelections(t *testing.T) {
	state := customization.DesiredState{
		Profile:    "developer",
		Purposes:   []string{"development"},
		Desktop:    "desktop.hyprland",
		Editor:     "editor.neovim",
		Additional: []string{"desktop.gnome"},
	}
	report := detection.Report{System: detection.SystemReport{OS: detection.OSDetails{ID: "arch", Name: "Arch Linux"}}}
	registry, err := components.LoadFromDir("../../components/manifests")
	if err != nil {
		t.Fatalf("load registry: %v", err)
	}
	plan, err := BuildPlanFromState(state, registry, report)
	if err != nil {
		t.Fatalf("build plan unexpectedly failed: %v", err)
	}
	if plan.Status == "ready" {
		t.Fatal("expected conflicting selection to block the plan")
	}
	if len(plan.Errors) == 0 {
		t.Fatal("expected plan errors for conflicts")
	}
}

func TestBuildPlanDetectsNonArchHost(t *testing.T) {
	cfg := config.Config{
		Profile: config.ProfileConfig{Name: "developer", Purposes: []string{"development"}},
		Desktop: config.DesktopConfig{Environment: "kde"},
	}
	report := detection.Report{System: detection.SystemReport{OS: detection.OSDetails{ID: "ubuntu", Name: "Ubuntu"}}}
	registry, err := components.LoadFromDir("../../components/manifests")
	if err != nil {
		t.Fatalf("load registry: %v", err)
	}
	plan, err := BuildPlan(cfg, registry, report)
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}
	if plan.Installable {
		t.Fatal("expected non-Arch host to be non-installable")
	}
	if plan.Status != "blocked" {
		t.Fatalf("expected blocked status, got %q", plan.Status)
	}
}

func TestBuildPlanSelectsDesktopPortalPackageVariants(t *testing.T) {
	registry, err := components.LoadFromDir("../../components/manifests")
	if err != nil {
		t.Fatal(err)
	}
	report := detection.Report{System: detection.SystemReport{OS: detection.OSDetails{ID: "arch"}}}
	tests := []struct {
		desktop  string
		want     string
		unwanted string
	}{
		{desktop: "desktop.hyprland", want: "xdg-desktop-portal-hyprland", unwanted: "xdg-desktop-portal-gnome"},
		{desktop: "desktop.gnome", want: "xdg-desktop-portal-gnome", unwanted: "xdg-desktop-portal-hyprland"},
		{desktop: "desktop.kde", want: "xdg-desktop-portal-kde", unwanted: "xdg-desktop-portal-hyprland"},
		{desktop: "desktop.xfce", want: "xdg-desktop-portal-gtk", unwanted: "xdg-desktop-portal-hyprland"},
		{desktop: "desktop.none", want: "xdg-desktop-portal-gtk", unwanted: "xdg-desktop-portal-hyprland"},
	}
	for _, test := range tests {
		t.Run(test.desktop, func(t *testing.T) {
			plan, err := BuildPlanFromState(customization.DesiredState{
				Profile: "developer", Purposes: []string{"development"}, Desktop: test.desktop,
			}, registry, report)
			if err != nil {
				t.Fatal(err)
			}
			if !sliceContains(plan.Packages, test.want) || sliceContains(plan.Packages, test.unwanted) {
				t.Fatalf("unexpected portal packages for %s: %v", test.desktop, plan.Packages)
			}
		})
	}
}

func TestCodeOSSManifestNamesOfficialArchEdition(t *testing.T) {
	registry, err := components.LoadFromDir("../../components/manifests")
	if err != nil {
		t.Fatal(err)
	}
	manifest, ok := registry.Get("editor.vscode")
	if !ok {
		t.Fatal("Code - OSS manifest is missing")
	}
	if manifest.Name != "Code - OSS" || manifest.Metadata["edition"] != "Code - OSS" {
		t.Fatalf("official Arch code package must be identified as Code - OSS: %+v", manifest)
	}
}

func TestBuildPlanBlocksAndIdentifiesMissingSelectedManifest(t *testing.T) {
	state := customization.DesiredState{
		Profile: "developer",
		Theme:   "appearance.tokyonight",
	}
	report := detection.Report{System: detection.SystemReport{OS: detection.OSDetails{ID: "arch"}}}
	plan, err := BuildPlanFromState(state, components.NewRegistry(), report)
	if err != nil {
		t.Fatal(err)
	}
	if plan.Installable || plan.Status != "blocked" {
		t.Fatalf("missing manifest must block installation: %+v", plan)
	}
	if !sliceContains(plan.Errors, "component manifest not found in registry: appearance.tokyonight") {
		t.Fatalf("expected a clear missing-manifest error, got %#v", plan.Errors)
	}
	if len(plan.Components) != 1 || plan.Components[0].ID != "appearance.tokyonight" || plan.Components[0].Status != "blocked" {
		t.Fatalf("missing selected component must remain visible in the plan: %+v", plan.Components)
	}
}

func TestTokyoNightHasItsOwnManifestWithoutThemeSubstitution(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Appearance.Theme = "tokyo-night"
	report := detection.Report{System: detection.SystemReport{OS: detection.OSDetails{ID: "arch"}}}
	registry, err := components.LoadFromDir("../../components/manifests")
	if err != nil {
		t.Fatalf("load registry: %v", err)
	}
	plan, err := BuildPlan(cfg, registry, report)
	if err != nil {
		t.Fatalf("build plan: %v", err)
	}
	foundTokyoNight := false
	foundCatppuccin := false
	for _, component := range plan.Components {
		foundTokyoNight = foundTokyoNight || component.ID == "appearance.tokyonight"
		foundCatppuccin = foundCatppuccin || component.ID == "appearance.catppuccin"
	}
	if !foundTokyoNight || foundCatppuccin {
		t.Fatalf("Tokyo Night must remain the selected theme without substitution: %+v", plan)
	}
	if len(plan.Errors) != 0 {
		t.Fatalf("Tokyo Night manifest should validate without planner errors: %#v", plan.Errors)
	}
}

func sliceContains(items []string, want string) bool {
	for _, item := range items {
		if item == want {
			return true
		}
	}
	return false
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}
