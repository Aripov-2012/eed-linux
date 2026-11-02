package testdrive

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"eed/internal/components"
	"eed/internal/config"
	"eed/internal/detection"
	"eed/internal/orchestrator"
	"eed/internal/packageinfo"
	"eed/internal/snapshot"
	"eed/internal/storage"
)

func testDependencies(hostOS string, warnings []string, storageErr error, capability snapshot.Capability) (orchestrator.Dependencies, *int) {
	cfg := config.DefaultConfig()
	cfg.Profile = config.ProfileConfig{Name: "test"}
	cfg.Desktop = config.DesktopConfig{Environment: "none", DisplayManager: "none"}
	cfg.Developer = config.DeveloperConfig{Git: true}
	cfg.Editor = config.EditorConfig{Name: "none"}
	cfg.Appearance = config.AppearanceConfig{Theme: "default"}

	registry := components.NewRegistry()
	if err := registry.Add(components.Manifest{
		SchemaVersion: 1, ID: "developer.git", Name: "Git", Category: "developer",
		SupportedOS: []string{"arch"}, Packages: []string{"git"},
	}); err != nil {
		panic(err)
	}
	calls := 0
	return orchestrator.Dependencies{
		ConfigPath: "unused-test-config",
		LoadConfig: func(string) (config.Config, error) {
			calls++
			return cfg, nil
		},
		Detect: func(context.Context) (detection.Report, error) {
			calls++
			return detection.Report{
				System:   detection.SystemReport{OS: detection.OSDetails{ID: hostOS, Name: hostOS}},
				Warnings: append([]string(nil), warnings...),
			}, nil
		},
		LoadRegistry: func(string) (*components.Registry, error) {
			calls++
			return registry, nil
		},
		AnalyzeStore: func() (storage.Report, error) {
			calls++
			return storage.Report{
				SchemaVersion: "1",
				Filesystems:   []storage.Filesystem{{MountPoint: "/", Filesystem: "ext4", AvailableBytes: 1024}},
				Summary:       storage.Summary{RootFilesystem: "ext4"},
				Warnings:      append([]string(nil), warnings...),
			}, storageErr
		},
		ProbeSnapshot: func(context.Context) snapshot.Capability {
			calls++
			return capability
		},
	}, &calls
}

func TestRunInteractiveCompletesReadOnlyFlowAndShowsConfirmation(t *testing.T) {
	deps, calls := testDependencies("arch", nil, nil, snapshot.Capability{
		Available: true, ToolInstalled: true, ConfigurationExists: true, CreationSupported: true,
		Backend: "btrfs_snapper", Status: "configured_unverified",
	})
	var output bytes.Buffer
	choice, err := RunInteractive(context.Background(), deps, strings.NewReader("1\n"), &output)
	if err != nil {
		t.Fatal(err)
	}
	if choice != ChoiceFinish {
		t.Fatalf("expected finish choice, got %q", choice)
	}
	if *calls != 5 {
		t.Fatalf("expected only five read-only subsystem calls, got %d", *calls)
	}
	if !strings.Contains(output.String(), FinalConfirmation) {
		t.Fatalf("final confirmation was not displayed: %s", output.String())
	}
	if !strings.Contains(output.String(), "available_to_current_user=") {
		t.Fatalf("storage metrics should explain user-available capacity: %s", output.String())
	}
	if !strings.Contains(output.String(), "Package transaction execution: unavailable") ||
		!strings.Contains(output.String(), "blocks installation") {
		t.Fatalf("incomplete installation managers were not surfaced: %s", output.String())
	}
	if !strings.Contains(output.String(), "Test Drive performed no package, configuration, service, or snapshot changes.") {
		t.Fatalf("future installation actions and the read-only boundary should be explicit: %s", output.String())
	}
	for _, forbidden := range []string{"sudo", "apt install", "pacman -S", "systemctl enable", "systemctl disable", "snapper create"} {
		if strings.Contains(output.String(), forbidden) {
			t.Fatalf("test drive must not suggest executing modifying command %q: %s", forbidden, output.String())
		}
	}
}

func TestBuildNeverPlansSnapshotCreationOrInstallation(t *testing.T) {
	deps, calls := testDependencies("arch", nil, nil, snapshot.Capability{
		Available: true, ToolInstalled: true, ConfigurationExists: true, CreationSupported: true,
		Backend: "btrfs_snapper", Status: "configured_unverified",
	})
	report, err := Build(context.Background(), deps)
	if err != nil {
		t.Fatal(err)
	}
	if report.Preview.Changes.SnapshotCreationPlanned || report.Preview.SnapshotFacts.SnapshotCreationVerified ||
		report.Preview.SnapshotFacts.FullRollbackVerified {
		t.Fatalf("read-only Test Drive must not create or claim a verified snapshot: %+v", report.Preview.SnapshotFacts)
	}
	if report.Preview.Changes.InstalledPackagesKnown || report.Preview.Changes.ConfigurationFilesKnown ||
		report.Preview.Changes.ServiceChangesKnown {
		t.Fatalf("Test Drive must not claim installation changes were executed: %+v", report.Preview.Changes)
	}
	if len(report.Preview.Selected) != 1 || report.Preview.Selected[0] != "developer.git" ||
		len(report.Preview.Changes.PackageCandidates) != 1 || report.Preview.Changes.PackageCandidates[0] != "git" {
		t.Fatalf("test drive must preserve selected components without substitution: %+v", report.Preview)
	}
	if *calls != 5 {
		t.Fatalf("expected only read-only preview dependencies, got %d calls", *calls)
	}
}

func TestInteractiveReportShowsPackageStateAndAdditionalSizes(t *testing.T) {
	deps, _ := testDependencies("arch", nil, nil, snapshot.Capability{})
	deps.InspectPackages = func(context.Context, []string) (packageinfo.Report, error) {
		return packageinfo.Report{
			Status: "available", MetadataStatus: "available", InventoryKnown: true,
			Candidates: []packageinfo.Candidate{{
				Name: "git", State: packageinfo.StateNotInstalled, RepositoryStatus: "available",
			}},
			ResolvedPackages: []packageinfo.Package{{
				Name: "git", State: packageinfo.StateNotInstalled, DownloadBytes: 120, InstalledBytes: 340,
				DownloadSizeKnown: true, InstalledSizeKnown: true,
			}},
			TotalDownloadBytes: 120, TotalInstalledBytes: 340,
			AdditionalDownloadBytes: 120, AdditionalInstallBytes: 340, EstimateComplete: true,
			TotalDownloadKnown: true, TotalInstalledKnown: true,
			AdditionalDownloadKnown: true, AdditionalInstallKnown: true,
		}, nil
	}
	var output bytes.Buffer
	if _, err := RunInteractive(context.Background(), deps, strings.NewReader("1\n"), &output); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{
		"git: not_installed (repository=available",
		"total_footprint=340 bytes",
		"additional_download=120 bytes",
		"additional_install=340 bytes",
		"Installed package inventory: available",
		"Detected desktop: unknown",
		"Target desktop: none",
	} {
		if !strings.Contains(output.String(), expected) {
			t.Errorf("Test Drive output missing %q:\n%s", expected, output.String())
		}
	}
}

func TestTestDriveReportsUnknownEstimatesInsteadOfZero(t *testing.T) {
	deps, _ := testDependencies("arch", nil, nil, snapshot.Capability{})
	var output bytes.Buffer
	if _, err := RunInteractive(context.Background(), deps, strings.NewReader("1\n"), &output); err != nil {
		t.Fatal(err)
	}
	for _, expected := range []string{"total_footprint=unknown bytes", "additional_download=unknown bytes", "additional_install=unknown bytes"} {
		if !strings.Contains(output.String(), expected) {
			t.Errorf("unknown estimate should be explicit (%q):\n%s", expected, output.String())
		}
	}
	if strings.Contains(output.String(), "additional_download=0 bytes") ||
		strings.Contains(output.String(), "additional_install=0 bytes") {
		t.Fatalf("unknown inventory must not be presented as a zero cost: %s", output.String())
	}
}

func TestRunInteractiveConfigurationChoiceDoesNotWriteConfig(t *testing.T) {
	deps, calls := testDependencies("arch", nil, nil, snapshot.Capability{})
	var output bytes.Buffer
	choice, err := RunInteractive(context.Background(), deps, strings.NewReader("2\n"), &output)
	if err != nil {
		t.Fatal(err)
	}
	if choice != ChoiceConfiguration {
		t.Fatalf("expected configuration choice, got %q", choice)
	}
	if !strings.Contains(output.String(), "Run `eed wizard` separately") {
		t.Fatalf("expected explicit no-write return guidance: %s", output.String())
	}
	if *calls != 5 {
		t.Fatalf("unexpected subsystem calls: %d", *calls)
	}
}

func TestRunInteractiveTreatsEOFAsCancellation(t *testing.T) {
	deps, _ := testDependencies("arch", nil, nil, snapshot.Capability{})
	choice, err := RunInteractive(context.Background(), deps, strings.NewReader(""), &bytes.Buffer{})
	if err != nil {
		t.Fatal(err)
	}
	if choice != ChoiceExit {
		t.Fatalf("expected cancellation to exit, got %q", choice)
	}
}

func TestJSONReportIsNonInteractiveAndStructured(t *testing.T) {
	deps, calls := testDependencies("arch", nil, nil, snapshot.Capability{})
	report, err := Build(context.Background(), deps)
	if err != nil {
		t.Fatal(err)
	}
	var output bytes.Buffer
	if err := WriteJSON(&output, report); err != nil {
		t.Fatal(err)
	}
	var decoded Report
	if err := json.Unmarshal(output.Bytes(), &decoded); err != nil {
		t.Fatalf("invalid JSON: %v", err)
	}
	if decoded.FinalConfirmationMessage != FinalConfirmation {
		t.Fatalf("unexpected confirmation: %q", decoded.FinalConfirmationMessage)
	}
	if strings.Contains(output.String(), "Select an option") {
		t.Fatal("JSON mode must not prompt for input")
	}
	if *calls != 5 {
		t.Fatalf("expected only read-only subsystem calls, got %d", *calls)
	}
}

func TestUnsupportedOSAndMissingSnapshotDoNotConflateFilesystem(t *testing.T) {
	deps, _ := testDependencies("ubuntu", nil, nil, snapshot.Capability{
		Status: "snapper_missing", Reason: "snapper executable is not installed",
	})
	report, err := Build(context.Background(), deps)
	if err != nil {
		t.Fatal(err)
	}
	if report.Preview.Plan.Installable || report.Preview.Plan.Status != "blocked" {
		t.Fatalf("expected unsupported OS to block installation: %+v", report.Preview.Plan)
	}
	if report.Preview.Assessment.OverallStatus != "diagnostic_only" {
		t.Fatalf("unsupported host should have a diagnostic-only assessment: %s", report.Preview.Assessment.OverallStatus)
	}
	facts := report.Preview.SnapshotFacts
	if facts.FilesystemDetected != "ext4" || facts.SnapshotToolInstalled || facts.BackendConfigured || facts.SnapshotCreationSupported {
		t.Fatalf("snapshot absence was confused with filesystem detection: %+v", facts)
	}
	if facts.FullRollbackVerified {
		t.Fatal("full rollback must not be reported as verified")
	}
	if facts.Status != "unsupported_filesystem" {
		t.Fatalf("ext4 should be reported as unsupported for the configured snapshot backend: %+v", facts)
	}
	foundUnsupportedHostInfo := false
	for _, diagnostic := range report.Preview.Diagnostics {
		if strings.Contains(strings.ToLower(diagnostic.Reason), "not arch linux") {
			foundUnsupportedHostInfo = diagnostic.Severity == "info"
		}
	}
	if !foundUnsupportedHostInfo {
		t.Fatalf("unsupported-host preview should be informational, not a software defect: %+v", report.Preview.Diagnostics)
	}
	for _, warning := range report.Preview.Warnings {
		if strings.Contains(strings.ToLower(warning), "not arch linux") {
			t.Fatalf("expected unsupported-host behavior to be omitted from software warnings: %v", report.Preview.Warnings)
		}
	}
}

func TestWarningsAreDeduplicatedAndKeepSubsystemDiagnostics(t *testing.T) {
	deps, _ := testDependencies("arch", []string{"permission denied", "permission denied"}, nil, snapshot.Capability{})
	report, err := Build(context.Background(), deps)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	hasDetection, hasStorage := false, false
	for _, warning := range report.Preview.Warnings {
		if warning == "permission denied" {
			count++
		}
	}
	for _, diagnostic := range report.Preview.Diagnostics {
		if diagnostic.Reason == "permission denied" && !diagnostic.BlocksInstallation {
			hasDetection = hasDetection || diagnostic.Subsystem == "system detection"
			hasStorage = hasStorage || diagnostic.Subsystem == "storage"
		}
	}
	if count != 1 || !hasDetection || !hasStorage {
		t.Fatalf("expected one warning and diagnostics from both subsystems; warnings=%v diagnostics=%+v", report.Preview.Warnings, report.Preview.Diagnostics)
	}
	if len(report.Preview.Storage.Warnings) != 1 {
		t.Fatalf("storage report should deduplicate identical warnings: %#v", report.Preview.Storage.Warnings)
	}
	for _, diagnostic := range report.Preview.Diagnostics {
		if diagnostic.Severity == "" {
			t.Fatalf("diagnostic severity must be explicit: %+v", diagnostic)
		}
	}
	foundComponentContext := false
	for _, diagnostic := range report.Preview.Diagnostics {
		if diagnostic.Component == "developer.git" && diagnostic.Subsystem == "assessment" {
			foundComponentContext = true
		}
	}
	if !foundComponentContext {
		t.Fatalf("component warning should retain component and subsystem context: %+v", report.Preview.Diagnostics)
	}
}

func TestStoragePermissionFailureIsAnExplicitBlocker(t *testing.T) {
	deps, _ := testDependencies("arch", nil, errors.New("operation not permitted"), snapshot.Capability{})
	report, err := Build(context.Background(), deps)
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, diagnostic := range report.Preview.Diagnostics {
		if diagnostic.Subsystem == "storage" && diagnostic.BlocksInstallation && diagnostic.Reason == "operation not permitted" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected explicit blocking storage diagnostic: %+v", report.Preview.Diagnostics)
	}
	if len(report.Preview.Stages) != 7 || report.Preview.Stages[5].Status != orchestrator.StageUnavailable {
		t.Fatalf("expected storage failure to be reported without aborting test drive: %+v", report.Preview.Stages)
	}
}
