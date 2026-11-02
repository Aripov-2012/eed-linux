package compatibility

import (
	"testing"

	"eed/internal/components"
	"eed/internal/config"
	"eed/internal/detection"
	"eed/internal/planner"
)

func TestHyprlandSupportedVendorListAllowsSingleIntelGPU(t *testing.T) {
	registry, err := components.LoadFromDir("../../components/manifests")
	if err != nil {
		t.Fatal(err)
	}
	checks := gpuChecks(detection.Report{
		Hardware: detection.HardwareReport{GPUs: []detection.GPUInfo{{Vendor: "Intel", VendorID: "0x8086"}}},
	}, registry, []string{"desktop.hyprland"})
	for _, check := range checks {
		if check.Status == "incompatible" {
			t.Fatalf("a supported Intel GPU must satisfy the vendor set without requiring AMD/NVIDIA: %+v", checks)
		}
	}
}

func TestGPUCompatibilityDoesNotBlockWhenVendorCannotBeVerified(t *testing.T) {
	registry, err := components.LoadFromDir("../../components/manifests")
	if err != nil {
		t.Fatal(err)
	}
	checks := gpuChecks(detection.Report{
		Hardware: detection.HardwareReport{GPUs: []detection.GPUInfo{{Vendor: "unknown"}}},
	}, registry, []string{"desktop.hyprland"})
	if len(checks) != 1 || checks[0].Status != "unknown" {
		t.Fatalf("unknown vendor information should not be treated as a verified incompatibility: %+v", checks)
	}
}

func TestExplicitRequiredGPUVendorIsCheckedAgainstDetectedHardware(t *testing.T) {
	registry := components.NewRegistry()
	if err := registry.Add(components.Manifest{
		SchemaVersion: 1,
		ID:            "desktop.test",
		Name:          "Test desktop",
		Category:      "desktop",
		SupportedOS:   []string{"arch"},
		Compatibility: components.CompatibilityRequirements{
			RequiredGPUVendor: []string{"NVIDIA"},
		},
	}); err != nil {
		t.Fatal(err)
	}
	checks := gpuChecks(detection.Report{
		Hardware: detection.HardwareReport{GPUs: []detection.GPUInfo{{Vendor: "Intel", VendorID: "0x8086"}}},
	}, registry, []string{"desktop.test"})
	if len(checks) != 1 || checks[0].Status != "incompatible" {
		t.Fatalf("an explicit, verified unsatisfied requirement must block compatibility: %+v", checks)
	}
}

func TestArchCompatibilityIsSupported(t *testing.T) {
	cfg := config.Config{
		Profile: config.ProfileConfig{Name: "developer"},
		Desktop: config.DesktopConfig{Environment: "hyprland"},
	}
	registry, err := components.LoadFromDir("../../components/manifests")
	if err != nil {
		t.Fatalf("load registry: %v", err)
	}
	plan := planner.Plan{Status: "ready", Components: []planner.ComponentPlan{{ID: "desktop.hyprland"}}}
	report := detection.Report{System: detection.SystemReport{OS: detection.OSDetails{ID: "arch"}, Architecture: "x86_64"}}
	res, err := Assess(cfg, plan, registry, report)
	if err != nil {
		t.Fatalf("assess: %v", err)
	}
	if res.Status == "incompatible" {
		t.Fatalf("expected Arch compatibility check to pass, got %+v", res)
	}
}

func TestAMD64MatchesX8664ManifestArchitecture(t *testing.T) {
	registry, err := components.LoadFromDir("../../components/manifests")
	if err != nil {
		t.Fatalf("load registry: %v", err)
	}
	report := detection.Report{System: detection.SystemReport{Architecture: "amd64"}}
	checks := architectureChecks(report, registry, []string{"appearance.tokyonight"})
	for _, check := range checks {
		if check.Status == "incompatible" {
			t.Fatalf("amd64 should match x86_64 manifest architectures: %+v", checks)
		}
	}
}

func TestUbuntuIsDiagnosticOnly(t *testing.T) {
	cfg := config.Config{Profile: config.ProfileConfig{Name: "developer"}, Desktop: config.DesktopConfig{Environment: "hyprland"}}
	registry, err := components.LoadFromDir("../../components/manifests")
	if err != nil {
		t.Fatalf("load registry: %v", err)
	}
	plan := planner.Plan{Status: "ready", Components: []planner.ComponentPlan{{ID: "desktop.hyprland"}}}
	report := detection.Report{System: detection.SystemReport{OS: detection.OSDetails{ID: "ubuntu"}, Architecture: "x86_64"}}
	res, err := Assess(cfg, plan, registry, report)
	if err != nil {
		t.Fatalf("assess: %v", err)
	}
	if res.Status != "diagnostic_only" {
		t.Fatalf("expected Ubuntu to be explicitly flagged as diagnostic-only, got %q", res.Status)
	}
}
