package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"strings"
	"testing"

	"eed/internal/components"
	"eed/internal/config"
	"eed/internal/detection"
	"eed/internal/orchestrator"
	"eed/internal/snapshot"
	"eed/internal/storage"
)

func testDriveCommandDependencies(t *testing.T) (orchestrator.Dependencies, *int) {
	t.Helper()
	cfg := config.DefaultConfig()
	cfg.Profile = config.ProfileConfig{Name: "test"}
	cfg.Desktop = config.DesktopConfig{Environment: "none", DisplayManager: "none"}
	cfg.Developer = config.DeveloperConfig{Git: true}
	cfg.Editor = config.EditorConfig{Name: "none"}
	cfg.Appearance = config.AppearanceConfig{Theme: "default"}
	registry := components.NewRegistry()
	if err := registry.Add(components.Manifest{
		SchemaVersion: 1, ID: "developer.git", Name: "Git", Category: "developer", SupportedOS: []string{"arch"},
	}); err != nil {
		t.Fatal(err)
	}
	calls := 0
	deps := orchestrator.Dependencies{
		ConfigPath: "unused-test-config",
		LoadConfig: func(string) (config.Config, error) {
			calls++
			return cfg, nil
		},
		Detect: func(context.Context) (detection.Report, error) {
			calls++
			return detection.Report{System: detection.SystemReport{OS: detection.OSDetails{ID: "arch"}}}, nil
		},
		LoadRegistry: func(string) (*components.Registry, error) {
			calls++
			return registry, nil
		},
		AnalyzeStore: func() (storage.Report, error) {
			calls++
			return storage.Report{Filesystems: []storage.Filesystem{{MountPoint: "/", Filesystem: "ext4"}}, Summary: storage.Summary{RootFilesystem: "ext4"}}, nil
		},
		ProbeSnapshot: func(context.Context) snapshot.Capability {
			calls++
			return snapshot.Capability{Status: "snapper_missing", Reason: "snapper executable is not installed"}
		},
	}
	return deps, &calls
}

func TestRootCommandRegistersTestDrive(t *testing.T) {
	root := NewRootCmd()
	command, _, err := root.Find([]string{"test-drive"})
	if err != nil {
		t.Fatal(err)
	}
	if command == nil || command.Name() != "test-drive" {
		t.Fatalf("expected test-drive to be registered, got %v", command)
	}
}

func TestDetectionSummaryGroupsVirtualDevicesAndSpecialMounts(t *testing.T) {
	report := detection.Report{
		System: detection.SystemReport{
			BlockDevices: []detection.BlockDevice{
				{Name: "nvme0n1", Kind: "physical_disk", Transport: "nvme", SizeBytes: 512 * 1024 * 1024 * 1024, SizeKnown: true},
				{Name: "nvme0n1p1", Kind: "partition", IsPartition: true, Major: 259, Minor: 1, SizeBytes: 512 * 1024 * 1024, SizeKnown: true},
				{Name: "loop0", Kind: "loop", SizeBytes: 1000, SizeKnown: true},
			},
			Storage: []detection.MountInfo{
				{MountPoint: "/", Filesystem: "ext4", Category: "root", CapacityKnown: true, AvailableBytes: 20 * 1024 * 1024 * 1024},
				{MountPoint: "/boot/efi", Filesystem: "vfat", Category: "efi", Major: 259, Minor: 1, CapacityKnown: true},
				{MountPoint: "/snap/a/1", Filesystem: "squashfs", Category: "snap"},
				{MountPoint: "/snap/b/2", Filesystem: "squashfs", Category: "snap"},
				{MountPoint: "/proc", Filesystem: "proc", Category: "pseudo"},
				{MountPoint: "/var/lib/docker", Filesystem: "overlay", Category: "overlay"},
				{MountPoint: "/data", Filesystem: "xfs", Category: "local", CapacityKnown: true},
			},
		},
	}
	cmd := NewDetectCmd()
	var output bytes.Buffer
	cmd.SetOut(&output)
	printDeviceSummary(cmd, report, false)
	printMountSummary(cmd, report, false)
	summary := output.String()
	for _, expected := range []string{
		"nvme0n1: 512 GiB, nvme",
		"nvme0n1p1: 0.5 GiB, vfat (EFI), mounted at /boot/efi",
		"Loop devices: 1 (primarily Snap)",
		"Snap squashfs mounts: 2",
		"Pseudo filesystems: 1",
		"Docker/container overlays: 1",
		"xfs on /data",
	} {
		if !strings.Contains(summary, expected) {
			t.Errorf("summary missing %q:\n%s", expected, summary)
		}
	}
	for _, hidden := range []string{"/snap/a/1", "/snap/b/2", "loop0:"} {
		if strings.Contains(summary, hidden) {
			t.Errorf("default summary should group detailed item %q:\n%s", hidden, summary)
		}
	}

	output.Reset()
	printDeviceSummary(cmd, report, true)
	printMountSummary(cmd, report, true)
	detailed := output.String()
	for _, expected := range []string{"loop0", "/snap/a/1", "/snap/b/2", "/var/lib/docker"} {
		if !strings.Contains(detailed, expected) {
			t.Errorf("detailed output missing %q:\n%s", expected, detailed)
		}
	}
}

func TestTestDriveCommandRunsInteractiveReadOnlyWorkflow(t *testing.T) {
	deps, calls := testDriveCommandDependencies(t)
	cmd := NewTestDriveCmdWithDependencies(deps)
	cmd.SetArgs([]string{})
	cmd.SetIn(strings.NewReader("3\n"))
	var output bytes.Buffer
	cmd.SetOut(&output)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if *calls != 5 {
		t.Fatalf("expected only five read-only subsystem calls; got %d", *calls)
	}
	for _, expected := range []string{
		"EED Test Drive (read-only)",
		"Proposed installation summary",
		"Host assessment: needs_review",
		"Snapshot: status=unsupported_filesystem",
		"Future installation (not performed)",
		"TEST DRIVE MODE — NO SYSTEM CHANGES WILL BE MADE.",
		"3) Exit",
	} {
		if !strings.Contains(output.String(), expected) {
			t.Errorf("interactive command output missing %q:\n%s", expected, output.String())
		}
	}
	for _, forbidden := range []string{"pacman -S", "apt install", "sudo ", "systemctl enable"} {
		if strings.Contains(output.String(), forbidden) {
			t.Errorf("interactive workflow must not propose modifying operation %q", forbidden)
		}
	}
}

func TestTestDriveCommandEOFCancelsWorkflow(t *testing.T) {
	deps, calls := testDriveCommandDependencies(t)
	cmd := NewTestDriveCmdWithDependencies(deps)
	cmd.SetArgs([]string{})
	cmd.SetIn(strings.NewReader(""))
	var output bytes.Buffer
	cmd.SetOut(&output)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	if *calls != 5 {
		t.Fatalf("cancellation should only follow read-only subsystem calls; got %d", *calls)
	}
	if !strings.Contains(output.String(), "Select an option:") {
		t.Fatalf("expected workflow to display its menu before cancellation: %s", output.String())
	}
}

func TestTestDriveJSONCommandDoesNotPrompt(t *testing.T) {
	deps, calls := testDriveCommandDependencies(t)
	cmd := NewTestDriveCmdWithDependencies(deps)
	cmd.SetArgs([]string{"--json"})
	cmd.SetIn(strings.NewReader("input must not be read"))
	var output bytes.Buffer
	cmd.SetOut(&output)
	if err := cmd.Execute(); err != nil {
		t.Fatal(err)
	}
	var report struct {
		FinalConfirmationMessage string `json:"final_confirmation_message"`
		Preview                  struct {
			Mode string `json:"mode"`
		} `json:"preview"`
	}
	if err := json.Unmarshal(output.Bytes(), &report); err != nil {
		t.Fatalf("command did not emit JSON: %v\n%s", err, output.String())
	}
	if report.FinalConfirmationMessage != "TEST DRIVE MODE — NO SYSTEM CHANGES WILL BE MADE." {
		t.Fatalf("unexpected final confirmation: %q", report.FinalConfirmationMessage)
	}
	if report.Preview.Mode != "read_only_preview" || strings.Contains(output.String(), "Select an option") {
		t.Fatalf("JSON command should be non-interactive and read-only: %s", output.String())
	}
	if *calls != 5 {
		t.Fatalf("expected only five read-only subsystem calls; got %d", *calls)
	}
}
