package resources

import (
	"strings"
	"testing"

	"eed/internal/components"
	"eed/internal/detection"
	"eed/internal/planner"
)

func TestResourceAnalysisUsesMostRestrictiveMemoryRequirement(t *testing.T) {
	registry, err := components.LoadFromDir("../../components/manifests")
	if err != nil {
		t.Fatalf("load registry: %v", err)
	}
	registry.Add(components.Manifest{SchemaVersion: 1, ID: "demo.highmem", Name: "High mem", Category: "demo", SupportedOS: []string{"arch"}, SupportedArchitectures: []string{"x86_64"}, Requirements: components.Requirements{MinimumTotalRAMBytes: 8 * 1024 * 1024 * 1024}, Packages: []string{"demo"}})
	report := detection.Report{Hardware: detection.HardwareReport{Memory: detection.MemoryInfo{TotalBytes: 4 * 1024 * 1024 * 1024}}}
	res := Analyze(report, registry, planner.Plan{Components: []planner.ComponentPlan{{ID: "demo.highmem"}}})
	if res.RAM.Status != "insufficient" {
		t.Fatalf("expected memory threshold to fail, got %q", res.RAM.Status)
	}
}

func TestResourceAnalysisReportsMinimumRecommendedAndAvailableRAMSeparately(t *testing.T) {
	registry := components.NewRegistry()
	manifest := components.Manifest{
		SchemaVersion: 1, ID: "test.ram", Name: "RAM profile", Category: "test",
		SupportedOS: []string{"arch"},
		Requirements: components.Requirements{
			MinimumTotalRAMBytes: 4, RecommendedTotalRAMBytes: 8, MinimumFreeRAMBytes: 3,
		},
	}
	if err := registry.Add(manifest); err != nil {
		t.Fatal(err)
	}
	report := detection.Report{Hardware: detection.HardwareReport{
		Memory: detection.MemoryInfo{TotalBytes: 6, AvailableBytes: 2},
	}}
	result := Analyze(report, registry, planner.Plan{Components: []planner.ComponentPlan{{ID: manifest.ID, Selected: true}}})
	if result.RAM.MinimumTotalBytes != 4 || result.RAM.RecommendedTotalBytes != 8 ||
		result.RAM.MinimumAvailableBytes != 3 || result.RAM.TotalStatus != "sufficient" ||
		result.RAM.AvailableStatus != "insufficient" || result.RAM.RecommendedStatus != "below_recommended" ||
		result.RAM.Status != "insufficient" || len(result.Errors) == 0 || len(result.Warnings) == 0 {
		t.Fatalf("RAM result should preserve minimum/recommended/available distinctions: %+v", result)
	}
}

func TestResourceAnalysisInsufficientMinimumRAMBlocksWithActionableError(t *testing.T) {
	registry := components.NewRegistry()
	manifest := components.Manifest{
		SchemaVersion: 1, ID: "test.profile", Name: "Heavy profile", Category: "test",
		SupportedOS: []string{"arch"},
		Requirements: components.Requirements{
			MinimumTotalRAMBytes:     8 * 1024 * 1024 * 1024,
			RecommendedTotalRAMBytes: 16 * 1024 * 1024 * 1024,
		},
	}
	if err := registry.Add(manifest); err != nil {
		t.Fatal(err)
	}
	report := detection.Report{Hardware: detection.HardwareReport{
		Memory: detection.MemoryInfo{TotalBytes: 4 * 1024 * 1024 * 1024, AvailableBytes: 3 * 1024 * 1024 * 1024},
	}}
	result := Analyze(report, registry, planner.Plan{Components: []planner.ComponentPlan{{ID: manifest.ID, Selected: true}}})
	if result.RAM.Status != "insufficient" || result.RAM.TotalStatus != "insufficient" || len(result.Errors) == 0 {
		t.Fatalf("failing true minimum requirement must not be bypassed and must produce errors: %+v", result)
	}
}

func TestResourceAnalysisInsufficientRecommendedRAMWarnsWithoutBlocking(t *testing.T) {
	registry := components.NewRegistry()
	manifest := components.Manifest{
		SchemaVersion: 1, ID: "test.profile", Name: "Recommended profile", Category: "test",
		SupportedOS: []string{"arch"},
		Requirements: components.Requirements{
			MinimumTotalRAMBytes:     4 * 1024 * 1024 * 1024,
			RecommendedTotalRAMBytes: 8 * 1024 * 1024 * 1024,
		},
	}
	if err := registry.Add(manifest); err != nil {
		t.Fatal(err)
	}
	report := detection.Report{Hardware: detection.HardwareReport{
		Memory: detection.MemoryInfo{TotalBytes: 6 * 1024 * 1024 * 1024, AvailableBytes: 5 * 1024 * 1024 * 1024},
	}}
	result := Analyze(report, registry, planner.Plan{Components: []planner.ComponentPlan{{ID: manifest.ID, Selected: true}}})
	if result.RAM.TotalStatus != "sufficient" || result.RAM.RecommendedStatus != "below_recommended" ||
		result.RAM.Status != "recommended_resources_limited" || len(result.Errors) != 0 || len(result.Warnings) == 0 {
		t.Fatalf("insufficient recommended RAM should warn without producing blocking errors: %+v", result)
	}
	if result.Status == "insufficient" {
		t.Fatalf("recommended RAM warning must not block as insufficient: %q", result.Status)
	}
}

func TestResourceAnalysisEvaluatesSelectedProfileRequirements(t *testing.T) {
	registry := components.NewRegistry()
	lightweight := components.Manifest{
		SchemaVersion: 1, ID: "desktop.light", Name: "Light desktop", Category: "desktop",
		SupportedOS: []string{"arch"},
		Requirements: components.Requirements{
			MinimumTotalRAMBytes:     2 * 1024 * 1024 * 1024,
			RecommendedTotalRAMBytes: 4 * 1024 * 1024 * 1024,
		},
	}
	heavyweight := components.Manifest{
		SchemaVersion: 1, ID: "desktop.heavy", Name: "Heavy desktop", Category: "desktop",
		SupportedOS: []string{"arch"},
		Requirements: components.Requirements{
			MinimumTotalRAMBytes:     8 * 1024 * 1024 * 1024,
			RecommendedTotalRAMBytes: 16 * 1024 * 1024 * 1024,
		},
	}
	if err := registry.Add(lightweight); err != nil {
		t.Fatal(err)
	}
	if err := registry.Add(heavyweight); err != nil {
		t.Fatal(err)
	}
	report := detection.Report{Hardware: detection.HardwareReport{
		Memory: detection.MemoryInfo{TotalBytes: 4 * 1024 * 1024 * 1024, AvailableBytes: 3 * 1024 * 1024 * 1024},
	}}
	lightResult := Analyze(report, registry, planner.Plan{Components: []planner.ComponentPlan{{ID: lightweight.ID, Selected: true}}})
	if lightResult.RAM.Status != "sufficient" || len(lightResult.Errors) != 0 {
		t.Fatalf("4 GiB system should satisfy lightweight profile: %+v", lightResult.RAM)
	}
	heavyResult := Analyze(report, registry, planner.Plan{Components: []planner.ComponentPlan{{ID: heavyweight.ID, Selected: true}}})
	if heavyResult.RAM.Status != "insufficient" || len(heavyResult.Errors) == 0 {
		t.Fatalf("4 GiB system should fail heavyweight profile: %+v", heavyResult.RAM)
	}
}

func TestStorageEstimateCanBeIncomplete(t *testing.T) {
	report := detection.Report{System: detection.SystemReport{Storage: []detection.MountInfo{{MountPoint: "/", Filesystem: "ext4", AvailableBytes: 1000}}}}
	res := Analyze(report, nil, planner.Plan{})
	if res.Storage.Status != "estimate_unavailable" || res.Storage.AvailableBytes != 1000 {
		t.Fatalf("expected known capacity with unavailable estimate, got %+v", res.Storage)
	}
}

func TestStorageCapacityUsesRootWithoutDoubleCountingHome(t *testing.T) {
	report := detection.Report{System: detection.SystemReport{Storage: []detection.MountInfo{
		{MountPoint: "/", Filesystem: "ext4", AvailableBytes: 1200},
		{MountPoint: "/home/user", Filesystem: "ext4", AvailableBytes: 1200},
	}}}
	result := Analyze(report, nil, planner.Plan{})
	if result.Storage.AvailableBytes != 1200 {
		t.Fatalf("root and home on the same filesystem must not be summed: %+v", result.Storage)
	}
}

func TestStorageCapacityAvailableWithoutRootMount(t *testing.T) {
	report := detection.Report{System: detection.SystemReport{Storage: []detection.MountInfo{
		{MountPoint: "/home/user", Filesystem: "ext4", AvailableBytes: 1200},
	}}}
	result := Analyze(report, nil, planner.Plan{})
	if result.Storage.AvailableBytes != 1200 {
		t.Fatalf("expected fallback to readable home capacity, got %+v", result.Storage)
	}
}

func TestPackageSizeEstimateIncludesComponentAndPackageDependenciesOnce(t *testing.T) {
	registry := components.NewRegistry()
	dependency := components.Manifest{
		SchemaVersion: 1,
		ID:            "test.dependency",
		Name:          "Dependency",
		Category:      "test",
		SupportedOS:   []string{"arch"},
		Packages:      []string{"shared", "lib"},
		Resources: components.ResourceEstimate{PackageEstimates: []components.PackageSizeEstimate{
			{Package: "shared", Version: "1.0", AsOf: "2026-01-01", EstimatedDownloadBytes: 2, EstimatedInstalledBytes: 3, Confidence: "approximate", Source: "Arch package metadata"},
			{Package: "lib", Version: "1.0", AsOf: "2026-01-01", EstimatedDownloadBytes: 4, EstimatedInstalledBytes: 5, Confidence: "approximate", Source: "Arch package metadata"},
		}},
	}
	root := components.Manifest{
		SchemaVersion: 1,
		ID:            "test.root",
		Name:          "Application",
		Category:      "test",
		SupportedOS:   []string{"arch"},
		Dependencies:  []string{"test.dependency"},
		Packages:      []string{"app", "shared"},
		Resources: components.ResourceEstimate{PackageEstimates: []components.PackageSizeEstimate{
			{
				Package: "app", Version: "1.0", AsOf: "2026-01-01", EstimatedDownloadBytes: 10, EstimatedInstalledBytes: 20,
				Dependencies: []string{"lib>=2.0"}, Confidence: "approximate", Source: "Arch package metadata",
			},
			{Package: "shared", Version: "1.0", AsOf: "2026-01-01", EstimatedDownloadBytes: 2, EstimatedInstalledBytes: 3, Confidence: "approximate", Source: "Arch package metadata"},
		}},
	}
	for _, manifest := range []components.Manifest{dependency, root} {
		if err := registry.Add(manifest); err != nil {
			t.Fatalf("add %s: %v", manifest.ID, err)
		}
	}
	report := detection.Report{System: detection.SystemReport{
		OS:      detection.OSDetails{ID: "arch"},
		Storage: []detection.MountInfo{{MountPoint: "/", AvailableBytes: 100}},
	}}
	result := Analyze(report, registry, planner.Plan{Components: []planner.ComponentPlan{
		{ID: "test.root", Selected: true},
		{ID: "test.dependency", Selected: true},
	}})
	if !result.Storage.EstimateComplete || result.Storage.EstimatedInstallBytes != 28 ||
		result.Storage.EstimatedDownloadBytes != 16 {
		t.Fatalf("expected package and component dependencies counted once, got %+v", result.Storage)
	}
	if result.Storage.EstimateConfidence != "approximate" || len(result.Storage.EstimateSources) != 3 ||
		result.Storage.EstimateSources[0].Source != "Arch package metadata" {
		t.Fatalf("expected approximation confidence and metadata provenance: %+v", result.Storage)
	}
}

func TestMissingPackageSizeMetadataIsExplainedWithoutGuessing(t *testing.T) {
	registry := components.NewRegistry()
	manifest := components.Manifest{
		SchemaVersion: 1,
		ID:            "test.unestimated",
		Name:          "Unestimated",
		Category:      "test",
		SupportedOS:   []string{"arch"},
		Packages:      []string{"unknown-package"},
		Resources: components.ResourceEstimate{
			EstimateConfidence: "unavailable",
			EstimateNotes:      "pinned Arch repository metadata is unavailable",
		},
	}
	if err := registry.Add(manifest); err != nil {
		t.Fatal(err)
	}
	report := detection.Report{System: detection.SystemReport{
		OS:      detection.OSDetails{ID: "arch"},
		Storage: []detection.MountInfo{{MountPoint: "/", AvailableBytes: 100}},
	}}
	result := Analyze(report, registry, planner.Plan{Components: []planner.ComponentPlan{{ID: manifest.ID, Selected: true}}})
	if result.Storage.EstimatedInstallBytes != 0 || result.Storage.EstimatedDownloadBytes != 0 ||
		result.Storage.EstimateComplete || result.Storage.Status != "estimate_unavailable" {
		t.Fatalf("missing metadata must not become an invented size: %+v", result.Storage)
	}
	if !strings.Contains(strings.Join(result.Storage.EstimateNotes, "\n"), "unknown-package") ||
		!strings.Contains(strings.Join(result.Storage.EstimateNotes, "\n"), "incomplete size metadata") {
		t.Fatalf("missing package metadata should be explained: %+v", result.Storage.EstimateNotes)
	}
}

func TestPackageMetadataIsNotAppliedToAnotherArchitecture(t *testing.T) {
	registry := components.NewRegistry()
	manifest := components.Manifest{
		SchemaVersion: 1,
		ID:            "test.arch-specific",
		Name:          "Architecture-specific",
		Category:      "test",
		SupportedOS:   []string{"arch"},
		Packages:      []string{"arch-package"},
		Resources: components.ResourceEstimate{PackageEstimates: []components.PackageSizeEstimate{
			{
				Package: "arch-package", Architecture: "x86_64", Version: "1.0", AsOf: "2026-01-01",
				EstimatedDownloadBytes: 10, EstimatedInstalledBytes: 20,
				Confidence: "approximate", Source: "Arch package metadata",
			},
		}},
	}
	if err := registry.Add(manifest); err != nil {
		t.Fatal(err)
	}
	report := detection.Report{
		System: detection.SystemReport{
			OS:           detection.OSDetails{ID: "arch"},
			Architecture: "amd64",
			Storage:      []detection.MountInfo{{MountPoint: "/", AvailableBytes: 100}},
		},
	}

	result := Analyze(report, registry, planner.Plan{Components: []planner.ComponentPlan{{ID: manifest.ID, Selected: true}}})
	if result.Storage.EstimatedInstallBytes != 20 || result.Storage.EstimatedDownloadBytes != 10 ||
		result.Storage.EstimateConfidence != "approximate" {
		t.Fatalf("amd64 should match x86_64 package metadata: %+v", result.Storage)
	}
	report.System.Architecture = "aarch64"
	result = Analyze(report, registry, planner.Plan{Components: []planner.ComponentPlan{{ID: manifest.ID, Selected: true}}})
	if result.Storage.EstimatedInstallBytes != 0 || result.Storage.EstimatedDownloadBytes != 0 ||
		result.Storage.EstimateConfidence != "unavailable" {
		t.Fatalf("x86_64 metadata must not be used for aarch64: %+v", result.Storage)
	}
}

func TestArchManifestEstimatesAreNotAppliedToUbuntu(t *testing.T) {
	registry := components.NewRegistry()
	manifest := components.Manifest{
		SchemaVersion: 1, ID: "test.arch-only", Name: "Arch only", Category: "test",
		SupportedOS: []string{"arch"}, Packages: []string{"sample"},
		Resources: components.ResourceEstimate{PackageEstimates: []components.PackageSizeEstimate{
			{Package: "sample", Architecture: "amd64", Version: "1.0", AsOf: "2026-01-01", EstimatedDownloadBytes: 20, EstimatedInstalledBytes: 30, Confidence: "approximate", Source: "Arch metadata"},
		}},
	}
	if err := registry.Add(manifest); err != nil {
		t.Fatal(err)
	}
	report := detection.Report{System: detection.SystemReport{
		OS:           detection.OSDetails{ID: "ubuntu", Name: "Ubuntu"},
		Architecture: "amd64",
		Storage:      []detection.MountInfo{{MountPoint: "/", AvailableBytes: 1000}},
	}}
	result := Analyze(report, registry, planner.Plan{Components: []planner.ComponentPlan{{ID: manifest.ID, Selected: true}}})
	if result.Storage.EstimatedInstallBytes != 0 || result.Storage.EstimatedDownloadBytes != 0 ||
		result.Storage.EstimatedInstallKnown || result.Storage.EstimatedDownloadKnown ||
		len(result.Storage.EstimateSources) != 0 {
		t.Fatalf("Arch package metadata must not become Ubuntu estimates: %+v", result.Storage)
	}
}
