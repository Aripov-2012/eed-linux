package orchestrator

import (
	"context"
	"errors"
	"strings"
	"testing"

	"eed/internal/components"
	"eed/internal/config"
	"eed/internal/detection"
	"eed/internal/packageinfo"
	"eed/internal/portalconfig"
	"eed/internal/snapshot"
	"eed/internal/storage"
)

func testDependencies() Dependencies {
	registry := components.NewRegistry()
	if err := registry.Add(components.Manifest{
		SchemaVersion:          1,
		ID:                     "developer.git",
		Name:                   "Git",
		Category:               "developer",
		SupportedOS:            []string{"arch"},
		SupportedArchitectures: []string{"x86_64"},
		Packages:               []string{"git"},
	}); err != nil {
		panic(err)
	}
	return Dependencies{
		LoadConfig: func(string) (config.Config, error) {
			return config.DefaultConfig(), nil
		},
		Detect: func(context.Context) (detection.Report, error) {
			return detection.Report{
				System: detection.SystemReport{OS: detection.OSDetails{ID: "arch", Name: "Arch Linux", IsSupportedTarget: true}},
			}, nil
		},
		LoadRegistry: func(string) (*components.Registry, error) {
			return registry, nil
		},
		AnalyzeStore: func() (storage.Report, error) {
			return storage.Report{SchemaVersion: "1", Filesystems: []storage.Filesystem{{MountPoint: "/", Filesystem: "ext4", AvailableBytes: 1024}}}, nil
		},
		ProbeSnapshot: func(context.Context) snapshot.Capability {
			return snapshot.Capability{Available: false, Backend: "btrfs_snapper", Status: "unsupported_filesystem", Reason: "root is ext4"}
		},
	}
}

func TestBuildPreviewRunsReadOnlyStagesAndSurfacesLimitations(t *testing.T) {
	preview, err := BuildPreview(context.Background(), testDependencies())
	if err != nil {
		t.Fatal(err)
	}
	if err := preview.Validate(); err != nil {
		t.Fatal(err)
	}
	if preview.Mode != "read_only_preview" || preview.Profile == "" {
		t.Fatalf("unexpected preview identity: %+v", preview)
	}
	if len(preview.Stages) != 7 {
		t.Fatalf("expected 7 completed preflight stages, got %d", len(preview.Stages))
	}
	if len(preview.Blockers) == 0 {
		t.Fatal("expected unavailable execution engines to be reported as blockers")
	}
	if preview.Changes.FullSystemUpgrade || preview.Changes.SnapshotCreationPlanned {
		t.Fatal("preview must not claim system modifications")
	}
	if preview.Snapshot.Available {
		t.Fatal("test snapshot backend should be unavailable")
	}
}

func TestBuildPreviewStopsAtDetectionFailure(t *testing.T) {
	deps := testDependencies()
	deps.Detect = func(context.Context) (detection.Report, error) {
		return detection.Report{}, errors.New("fixture failure")
	}
	preview, err := BuildPreview(context.Background(), deps)
	if err == nil {
		t.Fatal("expected detection failure")
	}
	if len(preview.Stages) != 2 || preview.Stages[1].Status != StageBlocked {
		t.Fatalf("expected failed detection stage to be recorded: %+v", preview.Stages)
	}
}

func TestBuildPreviewHonorsCancellation(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	preview, err := BuildPreview(ctx, testDependencies())
	if err == nil {
		t.Fatal("expected cancelled preview to stop")
	}
	if len(preview.Stages) != 1 || preview.Stages[0].Status != StageBlocked {
		t.Fatalf("expected cancellation stage record: %+v", preview.Stages)
	}
}

func TestSnapshotFactsDistinguishUnsupportedAndUnverifiedStates(t *testing.T) {
	tests := []struct {
		name       string
		filesystem string
		capability snapshot.Capability
		wantStatus string
	}{
		{
			name:       "unsupported ext4",
			filesystem: "ext4",
			capability: snapshot.Capability{Status: "snapper_missing"},
			wantStatus: "unsupported_filesystem",
		},
		{
			name:       "snapper missing",
			filesystem: "btrfs",
			capability: snapshot.Capability{Status: "snapper_missing"},
			wantStatus: "snapshot_tool_missing",
		},
		{
			name:       "backend not configured",
			filesystem: "btrfs",
			capability: snapshot.Capability{ToolInstalled: true, Status: "configuration_missing"},
			wantStatus: "backend_not_configured",
		},
		{
			name:       "creation unsupported",
			filesystem: "btrfs",
			capability: snapshot.Capability{ToolInstalled: true, ConfigurationExists: true},
			wantStatus: "snapshot_creation_unsupported",
		},
		{
			name:       "rollback unverified",
			filesystem: "btrfs",
			capability: snapshot.Capability{ToolInstalled: true, ConfigurationExists: true, CreationSupported: true},
			wantStatus: "full_rollback_unverified",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			facts := assessSnapshotFacts(test.filesystem, test.capability)
			if facts.Status != test.wantStatus || facts.FullRollbackVerified {
				t.Fatalf("unexpected snapshot facts: %+v", facts)
			}
			if facts.SnapshotCreationVerified {
				t.Fatalf("a read-only probe must not claim snapshot creation was verified: %+v", facts)
			}
			if facts.Reason == "" {
				t.Fatal("snapshot state should explain its status")
			}
		})
	}
}

func TestSnapshotFactsReportLVMThinAndSeparateBtrfsMountsWithoutClaimingProtection(t *testing.T) {
	lvmFacts := assessSnapshotFactsWithStorage("ext4", snapshot.Capability{}, storage.Report{
		Filesystems: []storage.Filesystem{{MountPoint: "/", Filesystem: "ext4", LVMLogicalVolume: true, LVMThin: true}},
	}, "kvm")
	if lvmFacts.Status != "lvm_thin_unmanaged" || !lvmFacts.LVMThinDetected ||
		lvmFacts.EEDManagedFilesystemSnapshot || !lvmFacts.ExternalVMSnapshotSuggested {
		t.Fatalf("thin LVM and optional external VM snapshot must not be conflated: %+v", lvmFacts)
	}

	btrfsFacts := assessSnapshotFactsWithStorage("btrfs", snapshot.Capability{
		ToolInstalled: true, ConfigurationExists: true, CreationSupported: true,
	}, storage.Report{
		BtrfsLayoutInspected: true,
		BtrfsSeparateMounts:  []string{"/home"},
	}, "")
	if btrfsFacts.Status != "full_rollback_unverified" || !btrfsFacts.BtrfsLayoutInspected ||
		len(btrfsFacts.BtrfsSeparateMounts) != 1 || btrfsFacts.EEDManagedFilesystemSnapshot {
		t.Fatalf("Btrfs mount layout should remain visible without claiming verified rollback: %+v", btrfsFacts)
	}
	if !strings.Contains(btrfsFacts.Reason, "/home") {
		t.Fatalf("separate Btrfs mounts should be called out: %+v", btrfsFacts)
	}

	ext4Facts := assessSnapshotFactsWithStorage("ext4", snapshot.Capability{}, storage.Report{
		Filesystems: []storage.Filesystem{{MountPoint: "/", Filesystem: "ext4"}},
	}, "")
	if ext4Facts.Status != "unsupported_filesystem" || ext4Facts.SnapshotModel != "limited_or_config_specific" ||
		ext4Facts.EEDManagedFilesystemSnapshot {
		t.Fatalf("plain ext4 should be identified as limited/unsupported without protection: %+v", ext4Facts)
	}
}

func TestBuildPreviewPreservesDiagnosticOnlyUbuntuTarget(t *testing.T) {
	deps := testDependencies()
	deps.LoadRegistry = func(string) (*components.Registry, error) {
		return components.LoadFromDir("../../components/manifests")
	}
	deps.Detect = func(context.Context) (detection.Report, error) {
		return detection.Report{System: detection.SystemReport{OS: detection.OSDetails{ID: "ubuntu", Name: "Ubuntu"}}}, nil
	}
	preview, err := BuildPreview(context.Background(), deps)
	if err != nil {
		t.Fatal(err)
	}
	if preview.Plan.Installable || preview.Assessment.OverallStatus != "diagnostic_only" {
		t.Fatalf("unsupported Ubuntu must remain diagnostic-only and non-installable: plan=%+v assessment=%+v", preview.Plan, preview.Assessment)
	}
	if len(preview.Summary.Incompatible) == 0 {
		t.Fatalf("report should explain why Ubuntu is not a valid installation target: %+v", preview.Summary)
	}
}

func TestBuildPreviewUsesUbuntuInventoryAndSeparatesDetectedTargetDesktop(t *testing.T) {
	deps := testDependencies()
	deps.LoadConfig = func(string) (config.Config, error) {
		cfg := config.DefaultConfig()
		cfg.Desktop.Environment = "desktop.hyprland"
		return cfg, nil
	}
	deps.Detect = func(context.Context) (detection.Report, error) {
		return detection.Report{System: detection.SystemReport{
			OS:      detection.OSDetails{ID: "ubuntu", Name: "Ubuntu"},
			Desktop: "GNOME",
		}}, nil
	}
	archCalled := false
	ubuntuCalled := false
	deps.InspectPackages = func(context.Context, []string) (packageinfo.Report, error) {
		archCalled = true
		return packageinfo.Report{}, nil
	}
	deps.InspectPackagesForOS = func(_ context.Context, osID string, packages []string) (packageinfo.Report, error) {
		if osID != "ubuntu" {
			t.Fatalf("expected Ubuntu-specific inspection, got %q", osID)
		}
		ubuntuCalled = true
		return packageinfo.Report{
			Status: "available", InventoryKnown: true, Candidates: []packageinfo.Candidate{},
		}, nil
	}

	preview, err := BuildPreview(context.Background(), deps)
	if err != nil {
		t.Fatal(err)
	}
	if !ubuntuCalled || archCalled {
		t.Fatalf("Ubuntu must use only its native inspector: ubuntu=%t arch=%t", ubuntuCalled, archCalled)
	}
	if preview.DetectedDesktop != "GNOME" || preview.TargetDesktop != "desktop.hyprland" {
		t.Fatalf("detected and selected desktops must be distinct: detected=%q target=%q",
			preview.DetectedDesktop, preview.TargetDesktop)
	}
	if !preview.Changes.InstalledPackagesKnown {
		t.Fatal("package inventory should reflect the native adapter's known dpkg inventory")
	}
}

func TestBuildPreviewPlansPortalsThroughInjectedAdapter(t *testing.T) {
	deps := testDependencies()
	registry := components.NewRegistry()
	for _, manifest := range []components.Manifest{
		{
			SchemaVersion: 1, ID: "system.portals", Name: "Desktop Portals",
			Category: "system", SupportedOS: []string{"arch"},
			PackageVariants: map[string][]string{"desktop.hyprland": {"xdg-desktop-portal", "xdg-desktop-portal-hyprland", "xdg-desktop-portal-gtk"}},
		},
		{
			SchemaVersion: 1, ID: "developer.git", Name: "Git",
			Category: "developer", SupportedOS: []string{"arch"},
			Packages: []string{"git"},
		},
	} {
		if err := registry.Add(manifest); err != nil {
			t.Fatal(err)
		}
	}
	deps.LoadRegistry = func(string) (*components.Registry, error) { return registry, nil }
	deps.LoadConfig = func(string) (config.Config, error) {
		cfg := config.DefaultConfig()
		cfg.Desktop.Environment = "desktop.hyprland"
		return cfg, nil
	}
	plannedDesktop := ""
	deps.PlanPortals = func(desktop string, _ map[string]bool) (portalconfig.Plan, error) {
		plannedDesktop = desktop
		return portalconfig.Plan{Desktop: desktop, Status: "proposal"}, nil
	}

	preview, err := BuildPreview(context.Background(), deps)
	if err != nil {
		t.Fatal(err)
	}
	if plannedDesktop != "desktop.hyprland" || preview.PortalPlan == nil || preview.PortalPlan.Desktop != plannedDesktop {
		t.Fatalf("portal planning should use configured desktop via injected adapter: desktop=%q plan=%+v", plannedDesktop, preview.PortalPlan)
	}
	if !hasStage(preview.Stages, "plan_portals", StageCompleted) {
		t.Fatalf("portal planning stage missing: %+v", preview.Stages)
	}
	if len(preview.Plan.PackageStatuses) != 1 || preview.Plan.PackageStatuses[0].State != packageinfo.StateUnknown {
		t.Fatalf("unavailable package inventory should explicitly retain unknown status: %+v", preview.Plan.PackageStatuses)
	}
}

func TestBuildPreviewReportsPackageInspectionFailureWithoutAborting(t *testing.T) {
	deps := testDependencies()
	deps.InspectPackages = func(context.Context, []string) (packageinfo.Report, error) {
		return packageinfo.Report{}, errors.New("fixture inventory failure")
	}
	preview, err := BuildPreview(context.Background(), deps)
	if err != nil {
		t.Fatalf("package diagnostics must not abort the read-only preview: %v", err)
	}
	if preview.PackageReport.Status != "unavailable" || len(preview.PackageReport.Errors) == 0 {
		t.Fatalf("inspection failure must remain visible in package report: %+v", preview.PackageReport)
	}
	if !hasStage(preview.Stages, "inspect_packages", StageUnavailable) {
		t.Fatalf("inspection failure should mark the stage unavailable: %+v", preview.Stages)
	}
}

func TestBuildPreviewMarksUnavailableInventoryStageWithoutError(t *testing.T) {
	deps := testDependencies()
	deps.InspectPackages = func(_ context.Context, packages []string) (packageinfo.Report, error) {
		return packageinfo.UnavailableReport(packages, "unavailable", "local package database could not be read"), nil
	}
	preview, err := BuildPreview(context.Background(), deps)
	if err != nil {
		t.Fatal(err)
	}
	if preview.PackageReport.Status != "unavailable" ||
		!hasStage(preview.Stages, "inspect_packages", StageUnavailable) {
		t.Fatalf("unavailable package report must not be presented as a completed inspection: report=%+v stages=%+v", preview.PackageReport, preview.Stages)
	}
}

func TestBuildPreviewRetainsPackageInventorySizesAfterAssessment(t *testing.T) {
	deps := testDependencies()
	deps.InspectPackages = func(context.Context, []string) (packageinfo.Report, error) {
		return packageinfo.Report{
			Status: "available", MetadataStatus: "available", ProviderResolutionStatus: "partial",
			Candidates:         []packageinfo.Candidate{{Name: "git", State: packageinfo.StateNotInstalled, RepositoryStatus: "available"}},
			ResolvedPackages:   []packageinfo.Package{{Name: "git", State: packageinfo.StateNotInstalled, DownloadBytes: 120, InstalledBytes: 340, DownloadSizeKnown: true, InstalledSizeKnown: true}},
			TotalDownloadBytes: 120, TotalInstalledBytes: 340,
			AdditionalDownloadBytes: 120, AdditionalInstallBytes: 340, EstimateComplete: true,
			InventoryKnown: true, TotalDownloadKnown: true, TotalInstalledKnown: true,
			AdditionalDownloadKnown: true, AdditionalInstallKnown: true,
		}, nil
	}
	preview, err := BuildPreview(context.Background(), deps)
	if err != nil {
		t.Fatal(err)
	}
	resource := preview.Assessment.Resources.Storage
	if !preview.Changes.InstalledPackagesKnown ||
		resource.TotalPackageFootprintBytes != 340 ||
		resource.AdditionalDownloadBytes != 120 ||
		resource.AdditionalInstallBytes != 340 {
		t.Fatalf("package inventory and estimates must survive assessment evaluation: changes=%+v resources=%+v", preview.Changes, resource)
	}
	if len(preview.Plan.PackageStatuses) != 1 || preview.Plan.PackageStatuses[0].Name != "git" {
		t.Fatalf("planner should expose candidate inventory states: %+v", preview.Plan.PackageStatuses)
	}
}

func hasStage(stages []Stage, id string, status StageStatus) bool {
	for _, stage := range stages {
		if stage.ID == id && stage.Status == status {
			return true
		}
	}
	return false
}
