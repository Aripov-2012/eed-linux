package assessment

import (
	"testing"

	"eed/internal/components"
	"eed/internal/config"
	"eed/internal/detection"
	"eed/internal/planner"
)

func TestAssessmentBlocksInsufficientResources(t *testing.T) {
	cfg := config.Config{Profile: config.ProfileConfig{Name: "developer"}, Desktop: config.DesktopConfig{Environment: "hyprland"}}
	registry, err := components.LoadFromDir("../../components/manifests")
	if err != nil {
		t.Fatalf("load registry: %v", err)
	}
	plan := planner.Plan{Status: "ready", Components: []planner.ComponentPlan{{ID: "desktop.hyprland"}}}
	report := detection.Report{
		System: detection.SystemReport{OS: detection.OSDetails{ID: "arch"}, Architecture: "x86_64"},
		Hardware: detection.HardwareReport{
			CPU:    detection.CPUInfo{LogicalCores: 2, PhysicalCores: 1, Architecture: "x86_64"},
			Memory: detection.MemoryInfo{TotalBytes: 1024},
		},
	}
	res, err := Evaluate(cfg, plan, registry, report)
	if err != nil {
		t.Fatalf("evaluate: %v", err)
	}
	if res.OverallStatus != "blocked" && res.OverallStatus != "needs_review" {
		t.Fatalf("expected blocked or needs_review from weak resources, got %q", res.OverallStatus)
	}
}

func TestUnsupportedOSIsDiagnosticOnlyNotACompatibilityDefect(t *testing.T) {
	cfg := config.Config{
		SchemaVersion: 1,
		Profile:       config.ProfileConfig{Name: "developer"},
		Desktop:       config.DesktopConfig{Environment: "none", DisplayManager: "none"},
		Editor:        config.EditorConfig{Name: "none"},
		Appearance:    config.AppearanceConfig{Theme: "default"},
	}
	registry, err := components.LoadFromDir("../../components/manifests")
	if err != nil {
		t.Fatal(err)
	}
	report := detection.Report{System: detection.SystemReport{OS: detection.OSDetails{ID: "ubuntu", Name: "Ubuntu"}}}
	result, err := Evaluate(cfg, planner.Plan{Status: "blocked", HostOS: "ubuntu"}, registry, report)
	if err != nil {
		t.Fatal(err)
	}
	if result.Compatibility.Status != "diagnostic_only" {
		t.Fatalf("unsupported OS should be diagnostic-only, got %+v", result.Compatibility)
	}
	if result.OverallStatus != "diagnostic_only" {
		t.Fatalf("expected explicit diagnostic-only assessment on Ubuntu, got %+v", result)
	}
}

func TestAssessmentReadyMeansHostChecksPassed(t *testing.T) {
	cfg := config.Config{
		SchemaVersion: 1,
		Profile:       config.ProfileConfig{Name: "developer"},
		Desktop:       config.DesktopConfig{Environment: "none", DisplayManager: "none"},
		Developer:     config.DeveloperConfig{Git: true},
		Editor:        config.EditorConfig{Name: "none"},
		Appearance:    config.AppearanceConfig{Theme: "default"},
	}
	registry := components.NewRegistry()
	if err := registry.Add(components.Manifest{
		SchemaVersion:          1,
		ID:                     "developer.git",
		Name:                   "Git",
		Category:               "developer",
		SupportedOS:            []string{"arch"},
		SupportedArchitectures: []string{"x86_64"},
		Packages:               []string{"git"},
		Resources: components.ResourceEstimate{PackageEstimates: []components.PackageSizeEstimate{
			{Package: "git", Version: "1.0", AsOf: "2026-01-01", EstimatedDownloadBytes: 10, EstimatedInstalledBytes: 20, Confidence: "approximate", Source: "test package metadata"},
		}},
	}); err != nil {
		t.Fatal(err)
	}
	report := detection.Report{
		System: detection.SystemReport{
			OS:           detection.OSDetails{ID: "arch", Name: "Arch Linux"},
			Architecture: "x86_64",
			Storage:      []detection.MountInfo{{MountPoint: "/", Filesystem: "ext4", AvailableBytes: 1000}},
		},
		Hardware: detection.HardwareReport{
			CPU:    detection.CPUInfo{LogicalCores: 4, PhysicalCores: 2},
			Memory: detection.MemoryInfo{TotalBytes: 8 * 1024 * 1024 * 1024},
		},
	}
	plan := planner.Plan{HostOS: "arch", Installable: true, Components: []planner.ComponentPlan{{ID: "developer.git", Selected: true}}}
	result, err := Evaluate(cfg, plan, registry, report)
	if err != nil {
		t.Fatal(err)
	}
	if result.OverallStatus != "ready" {
		t.Fatalf("expected host checks to be ready when all required facts are known: %+v", result)
	}
}
