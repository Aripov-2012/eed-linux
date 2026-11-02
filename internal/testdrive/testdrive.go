package testdrive

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"strings"

	"eed/internal/orchestrator"
	"eed/internal/packageinfo"
)

const FinalConfirmation = "TEST DRIVE MODE — NO SYSTEM CHANGES WILL BE MADE."

type Report struct {
	Preview                  orchestrator.Preview `json:"preview"`
	FinalConfirmationMessage string               `json:"final_confirmation_message"`
}

type Choice string

const (
	ChoiceFinish        Choice = "finish"
	ChoiceConfiguration Choice = "configuration"
	ChoiceExit          Choice = "exit"
)

func Build(ctx context.Context, deps orchestrator.Dependencies) (Report, error) {
	preview, err := orchestrator.BuildPreview(ctx, deps)
	if err != nil {
		return Report{}, fmt.Errorf("build test-drive report: %w", err)
	}
	return Report{Preview: preview, FinalConfirmationMessage: FinalConfirmation}, nil
}

func RunInteractive(ctx context.Context, deps orchestrator.Dependencies, input io.Reader, output io.Writer) (Choice, error) {
	report, err := Build(ctx, deps)
	if err != nil {
		return "", err
	}
	if err := render(output, report); err != nil {
		return "", err
	}

	reader := bufio.NewReader(input)
	for {
		if _, err := fmt.Fprintln(output, "\n1) Finish test drive\n2) Return to configuration\n3) Exit"); err != nil {
			return "", err
		}
		if _, err := fmt.Fprint(output, "Select an option: "); err != nil {
			return "", err
		}
		line, readErr := reader.ReadString('\n')
		if readErr != nil && readErr != io.EOF {
			return "", fmt.Errorf("read test-drive selection: %w", readErr)
		}
		switch strings.TrimSpace(line) {
		case "1":
			return ChoiceFinish, nil
		case "2":
			if _, err := fmt.Fprintln(output, "Test drive finished without changes. Run `eed wizard` separately to edit configuration."); err != nil {
				return "", err
			}
			return ChoiceConfiguration, nil
		case "3":
			return ChoiceExit, nil
		}
		if readErr == io.EOF {
			return ChoiceExit, nil
		}
		if _, err := fmt.Fprintln(output, "Invalid selection."); err != nil {
			return "", err
		}
	}
}

func WriteJSON(output io.Writer, report Report) error {
	encoder := json.NewEncoder(output)
	encoder.SetIndent("", "  ")
	if err := encoder.Encode(report); err != nil {
		return fmt.Errorf("encode test-drive report: %w", err)
	}
	return nil
}

func render(output io.Writer, report Report) error {
	preview := report.Preview
	if _, err := fmt.Fprintf(output, "EED Test Drive (read-only)\nProfile: %s\nHost: %s\nDetected desktop: %s\nTarget desktop: %s\nPlan status: %s\nHost assessment: %s\n",
		preview.Profile, preview.Plan.HostOS, valueOrUnknown(preview.DetectedDesktop), valueOrUnknown(preview.TargetDesktop),
		preview.Plan.Status, preview.Assessment.OverallStatus); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(output, "CPU: %s (%d logical / %d physical)\nMemory: total=%s bytes, available=%s bytes\n",
		valueOrUnknown(preview.DetectedCPU.Model), preview.DetectedCPU.LogicalCores, preview.DetectedCPU.PhysicalCores,
		knownBytes(preview.DetectedMemory.TotalBytes, preview.DetectedMemory.TotalBytes > 0),
		knownBytes(preview.DetectedMemory.AvailableBytes, preview.DetectedMemory.AvailableBytes > 0)); err != nil {
		return err
	}
	if _, err := fmt.Fprintf(output, "Compatibility: %s\nResources: CPU=%s, RAM=%s, storage=%s\n",
		preview.Assessment.Compatibility.Status,
		preview.Assessment.Resources.CPU.Status,
		preview.Assessment.Resources.RAM.Status,
		preview.Assessment.Resources.Storage.Status); err != nil {
		return err
	}
	ram := preview.Assessment.Resources.RAM
	if _, err := fmt.Fprintf(output, "RAM requirements: minimum total=%s bytes (%s), recommended total=%s bytes (%s), minimum available=%s bytes (%s)\n",
		knownBytes(ram.MinimumTotalBytes, ram.MinimumTotalBytes > 0), ram.TotalStatus,
		knownBytes(ram.RecommendedTotalBytes, ram.RecommendedTotalBytes > 0), ram.RecommendedStatus,
		knownBytes(ram.MinimumAvailableBytes, ram.MinimumAvailableBytes > 0), ram.AvailableStatus); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(output, "\nPackage managers (read-only detection):"); err != nil {
		return err
	}
	if len(preview.PackageManagers) > 0 {
		first := preview.PackageManagers[0]
		hostNative := "unknown"
		for _, manager := range preview.PackageManagers {
			if manager.HostNative {
				hostNative = manager.Name
				break
			}
		}
		if _, err := fmt.Fprintf(output, "  Host: %s (native manager: %s); target: %s (manager: %s)\n",
			first.HostDistribution, hostNative, first.ConfiguredTargetDistribution, first.TargetPackageManager); err != nil {
			return err
		}
	}
	if len(preview.PackageManagers) == 0 {
		if _, err := fmt.Fprintln(output, "  (unavailable)"); err != nil {
			return err
		}
	}
	for _, manager := range preview.PackageManagers {
		role := "not host-native"
		if manager.HostNative {
			role = "host-native"
		} else if manager.Name == manager.TargetPackageManager {
			role = "target manager"
		}
		version := manager.Version
		if version == "" {
			version = "version unavailable"
		}
		if _, err := fmt.Fprintf(output, "  - %s: %s, %s, adapter %s, %s\n",
			manager.Name, role, availability(manager.Available), manager.AdapterStatus, version); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(output, "\nInstalled packages:"); err != nil {
		return err
	}
	if !preview.PackageReport.InventoryKnown {
		if _, err := fmt.Fprintln(output, "  (inventory unknown)"); err != nil {
			return err
		}
	} else if len(preview.PackageReport.InstalledPackages) == 0 {
		if _, err := fmt.Fprintln(output, "  (none reported)"); err != nil {
			return err
		}
	} else {
		for _, pkg := range preview.PackageReport.InstalledPackages {
			if _, err := fmt.Fprintf(output, "  - %s %s\n", pkg.Name, valueOrUnknown(pkg.Version)); err != nil {
				return err
			}
		}
	}
	if _, err := fmt.Fprintln(output, "\nDetected GPUs:"); err != nil {
		return err
	}
	if len(preview.DetectedGPUs) == 0 {
		if _, err := fmt.Fprintln(output, "  (none detected by available read-only probes)"); err != nil {
			return err
		}
	}
	for _, gpu := range preview.DetectedGPUs {
		driver := "no bound driver detected"
		if gpu.DriverBound && strings.TrimSpace(gpu.Driver) != "" {
			driver = "driver " + strings.TrimSpace(gpu.Driver) + " bound"
		}
		if _, err := fmt.Fprintf(output, "  - %s %s (vendor %s, device %s; %s)\n",
			gpu.Vendor, gpu.Model, valueOrUnknown(gpu.VendorID), valueOrUnknown(gpu.DeviceID), driver); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(output, "\nBlock devices:"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(output, "Physical disks:"); err != nil {
		return err
	}
	diskCount := 0
	for _, device := range preview.BlockDevices {
		if device.IsPartition || (device.Kind != "" && device.Kind != "physical_disk") {
			continue
		}
		diskCount++
		transport := ""
		if device.Transport != "" && device.Transport != "unknown" {
			transport = ", " + device.Transport
		}
		if _, err := fmt.Fprintf(output, "  - %s: %s (%s, %s bytes)\n",
			device.Name, device.Path, "physical disk"+transport, knownBytes(device.SizeBytes, device.SizeKnown)); err != nil {
			return err
		}
	}
	if diskCount == 0 {
		if _, err := fmt.Fprintln(output, "  (none detected)"); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(output, "Partitions:"); err != nil {
		return err
	}
	partitionCount, loopCount, mapperCount, virtualCount := 0, 0, 0, 0
	for _, device := range preview.BlockDevices {
		if device.IsPartition || device.Kind == "partition" {
			partitionCount++
			if _, err := fmt.Fprintf(output, "  - %s: %s bytes\n", device.Name,
				knownBytes(device.SizeBytes, device.SizeKnown)); err != nil {
				return err
			}
		}
		switch device.Kind {
		case "loop":
			loopCount++
		case "device_mapper":
			mapperCount++
		case "virtual":
			virtualCount++
		}
	}
	if partitionCount == 0 {
		if _, err := fmt.Fprintln(output, "  (none detected)"); err != nil {
			return err
		}
	}
	if loopCount+mapperCount+virtualCount > 0 {
		if _, err := fmt.Fprintln(output, "Virtual devices:"); err != nil {
			return err
		}
		for _, group := range []struct {
			label string
			count int
		}{
			{label: "Loop devices (primarily Snap)", count: loopCount},
			{label: "Device-mapper devices", count: mapperCount},
			{label: "Other virtual devices", count: virtualCount},
		} {
			if group.count > 0 {
				if _, err := fmt.Fprintf(output, "  - %s: %d\n", group.label, group.count); err != nil {
					return err
				}
			}
		}
	}
	if _, err := fmt.Fprintln(output, "\nMounted filesystems:"); err != nil {
		return err
	}
	if len(preview.DetectedStorage) == 0 {
		if _, err := fmt.Fprintln(output, "  (not detected)"); err != nil {
			return err
		}
	}
	groups := map[string]int{}
	for _, mount := range preview.DetectedStorage {
		switch mount.Category {
		case "root", "home", "bind", "boot", "efi":
			name := mount.Category
			if name == "bind" {
				name = "home"
			}
			label := strings.ToUpper(name[:1]) + name[1:]
			if mount.Category == "efi" {
				label = "EFI"
			}
			if _, err := fmt.Fprintf(output, "  - %s: %s on %s, available=%s bytes",
				label, valueOrUnknown(mount.Filesystem), mount.MountPoint,
				knownBytes(mount.AvailableBytes, mount.CapacityKnown)); err != nil {
				return err
			}
			if mount.BindMount {
				if _, err := fmt.Fprint(output, " (bind mount; same filesystem)"); err != nil {
					return err
				}
			}
			if _, err := fmt.Fprintln(output); err != nil {
				return err
			}
		case "snap", "pseudo", "overlay", "fuse", "namespace", "virtual":
			groups[mount.Category]++
		default:
			if _, err := fmt.Fprintf(output, "  - %s: %s on %s, total=%s bytes, available=%s bytes\n",
				valueOrUnknown(mount.SourceDevice), valueOrUnknown(mount.Filesystem), mount.MountPoint,
				knownBytes(mount.TotalBytes, mount.CapacityKnown), knownBytes(mount.AvailableBytes, mount.CapacityKnown)); err != nil {
				return err
			}
		}
	}
	for _, group := range []struct {
		category string
		label    string
	}{
		{category: "snap", label: "Snap squashfs mounts"},
		{category: "pseudo", label: "Pseudo filesystems"},
		{category: "overlay", label: "Docker/container overlays"},
		{category: "fuse", label: "FUSE/GVFS mounts"},
		{category: "namespace", label: "Namespace mounts"},
		{category: "virtual", label: "Other virtual filesystems"},
	} {
		if groups[group.category] > 0 {
			if _, err := fmt.Fprintf(output, "  - %s: %d\n", group.label, groups[group.category]); err != nil {
				return err
			}
		}
	}
	if _, err := fmt.Fprintln(output, "\nSelected components:"); err != nil {
		return err
	}
	if len(preview.Selected) == 0 {
		if _, err := fmt.Fprintln(output, "  (none)"); err != nil {
			return err
		}
	}
	for _, component := range preview.Selected {
		if _, err := fmt.Fprintf(output, "  - %s\n", component); err != nil {
			return err
		}
	}
	totalBytes, freeBytes, availableBytes, storageKnown := rootStorageMetrics(preview)
	if _, err := fmt.Fprintf(output,
		"\nStorage: filesystem=%s, capacity=%s bytes, free=%s bytes, available_to_current_user=%s bytes\n"+
			"Installation size estimate: total_footprint=%s bytes, total_download=%s bytes, total_installed=%s bytes, additional_download=%s bytes, additional_install=%s bytes, confidence=%s, status=%s\n"+
			"Snapshot: status=%s, filesystem_supported=%t, tool_installed=%t, backend_configured=%t, creation_supported=%t, creation_verified=%t, full_rollback_verified=%t\n"+
			"  Snapshot diagnostic: %s\n",
		valueOrUnknown(preview.SnapshotFacts.FilesystemDetected),
		knownBytes(totalBytes, storageKnown), knownBytes(freeBytes, storageKnown), knownBytes(availableBytes, storageKnown),
		knownBytes(preview.Assessment.Resources.Storage.TotalPackageFootprintBytes, preview.Assessment.Resources.Storage.TotalPackageFootprintKnown),
		knownBytes(preview.Assessment.Resources.Storage.EstimatedDownloadBytes, preview.Assessment.Resources.Storage.EstimatedDownloadKnown),
		knownBytes(preview.Assessment.Resources.Storage.EstimatedInstallBytes, preview.Assessment.Resources.Storage.EstimatedInstallKnown),
		knownBytes(preview.Assessment.Resources.Storage.AdditionalDownloadBytes, preview.Assessment.Resources.Storage.AdditionalDownloadKnown),
		knownBytes(preview.Assessment.Resources.Storage.AdditionalInstallBytes, preview.Assessment.Resources.Storage.AdditionalInstallKnown),
		preview.Assessment.Resources.Storage.EstimateConfidence,
		preview.Assessment.Resources.Storage.Status,
		preview.SnapshotFacts.Status,
		preview.SnapshotFacts.FilesystemSupported,
		preview.SnapshotFacts.SnapshotToolInstalled,
		preview.SnapshotFacts.BackendConfigured,
		preview.SnapshotFacts.SnapshotCreationSupported,
		preview.SnapshotFacts.SnapshotCreationVerified,
		preview.SnapshotFacts.FullRollbackVerified,
		preview.SnapshotFacts.Reason); err != nil {
		return err
	}
	if preview.SnapshotFacts.ExternalVMSnapshotSuggested {
		if _, err := fmt.Fprintln(output, "  Optional safeguard: take an external VM snapshot through the hypervisor; it is not EED-managed."); err != nil {
			return err
		}
	}
	if len(preview.Storage.SkippedMounts) > 0 {
		if _, err := fmt.Fprintln(output, "Skipped optional mountpoints:"); err != nil {
			return err
		}
		for _, group := range preview.Storage.SkippedMounts {
			if _, err := fmt.Fprintf(output, "  - %s: %d", strings.ReplaceAll(group.Category, "_", " "), group.Count); err != nil {
				return err
			}
			if len(group.MountPoints) > 0 {
				if _, err := fmt.Fprintf(output, " (examples: %s)", strings.Join(group.MountPoints, ", ")); err != nil {
					return err
				}
			}
			if _, err := fmt.Fprintln(output); err != nil {
				return err
			}
		}
	}
	if err := renderSummarySection(output, "Detected", preview.Summary.Detected); err != nil {
		return err
	}
	if err := renderSummarySection(output, "Could not determine", preview.Summary.CouldNotDetermine); err != nil {
		return err
	}
	if err := renderSummarySection(output, "Incompatible with host", preview.Summary.Incompatible); err != nil {
		return err
	}
	if err := renderSummarySection(output, "Not implemented", preview.Summary.NotImplemented); err != nil {
		return err
	}
	if err := renderSummarySection(output, "Future installation (not performed)", preview.Summary.FutureInstallation); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(output, "\nProposed installation summary (not executed):"); err != nil {
		return err
	}
	if len(preview.Changes.PackageCandidates) == 0 {
		if _, err := fmt.Fprintln(output, "  Package candidates: (none)"); err != nil {
			return err
		}
	} else {
		if _, err := fmt.Fprintln(output, "  Package candidates:"); err != nil {
			return err
		}
		candidateByName := make(map[string]packageinfo.Candidate, len(preview.PackageReport.Candidates))
		for _, candidate := range preview.PackageReport.Candidates {
			candidateByName[candidate.Name] = candidate
		}
		for _, name := range preview.Changes.PackageCandidates {
			candidate, ok := candidateByName[name]
			if ok {
				if _, err := fmt.Fprintf(output, "    - %s: %s (repository=%s, installed=%s, available=%s)\n",
					name, candidate.State, candidate.RepositoryStatus,
					valueOrUnknown(candidate.InstalledVersion), valueOrUnknown(candidate.RepositoryVersion)); err != nil {
					return err
				}
				continue
			}
			if _, err := fmt.Fprintf(output, "    - %s: unknown\n", name); err != nil {
				return err
			}
		}
	}
	if preview.PortalPlan != nil {
		if _, err := fmt.Fprintf(output, "  Portal configuration proposal: desktop=%s, status=%s, path=%s\n",
			preview.PortalPlan.Desktop, preview.PortalPlan.Status, preview.PortalPlan.ConfigPath); err != nil {
			return err
		}
		if _, err := fmt.Fprintf(output, "    Package inventory: %s\n", preview.PortalPlan.PackageInventoryStatus); err != nil {
			return err
		}
		if preview.PortalPlan.PreserveExisting {
			if _, err := fmt.Fprintf(output, "    Existing user configuration preserved: %s\n",
				strings.Join(preview.PortalPlan.ExistingConfigPaths, ", ")); err != nil {
				return err
			}
		} else if _, err := fmt.Fprintf(output, "    Proposed content:\n%s", preview.PortalPlan.Content); err != nil {
			return err
		}
		if len(preview.PortalPlan.MissingPackages) > 0 {
			if _, err := fmt.Fprintf(output, "    Missing portal packages: %s\n", strings.Join(preview.PortalPlan.MissingPackages, ", ")); err != nil {
				return err
			}
		}
	}
	installedState := "unknown"
	if preview.Changes.InstalledPackagesKnown {
		installedState = "available"
	}
	if _, err := fmt.Fprintln(output,
		"  Installed package inventory: "+installedState,
		"  Package transaction execution: unavailable (not enabled in Test Drive)",
		"  Configuration manager: unavailable",
		"  Service manager: unavailable",
		"  Configuration file changes: unknown (manager unavailable)",
		"  Service changes: unknown (manager unavailable)",
		"  Full system upgrade: not planned",
		"  Snapshot creation: not planned"); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(output, "\nWarnings:"); err != nil {
		return err
	}
	if len(preview.Warnings) == 0 {
		if _, err := fmt.Fprintln(output, "  (none)"); err != nil {
			return err
		}
	}
	for _, warning := range preview.Warnings {
		if _, err := fmt.Fprintf(output, "  - %s\n", warning); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(output, "\nInstallation blockers:"); err != nil {
		return err
	}
	if len(preview.Blockers) == 0 {
		if _, err := fmt.Fprintln(output, "  (none)"); err != nil {
			return err
		}
	}
	for _, blocker := range preview.Blockers {
		if _, err := fmt.Fprintf(output, "  - %s\n", blocker); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintln(output, "\nDiagnostics:"); err != nil {
		return err
	}
	if len(preview.Diagnostics) == 0 {
		if _, err := fmt.Fprintln(output, "  (none)"); err != nil {
			return err
		}
	}
	for _, diagnostic := range preview.Diagnostics {
		impact := "does not block installation"
		if diagnostic.BlocksInstallation {
			impact = "blocks installation"
		}
		context := diagnostic.Subsystem
		if diagnostic.Component != "" {
			context += "/" + diagnostic.Component
		}
		if _, err := fmt.Fprintf(output, "  - [%s] %s: %s: %s; %s\n",
			strings.ToUpper(diagnostic.Severity), context, diagnostic.Operation, diagnostic.Reason, impact); err != nil {
			return err
		}
	}
	if _, err := fmt.Fprintf(output, "\n%s\n", report.FinalConfirmationMessage); err != nil {
		return err
	}
	return nil
}

func renderSummarySection(output io.Writer, title string, values []string) error {
	if _, err := fmt.Fprintf(output, "\n%s:\n", title); err != nil {
		return err
	}
	if len(values) == 0 {
		_, err := fmt.Fprintln(output, "  (none)")
		return err
	}
	for _, value := range values {
		if _, err := fmt.Fprintf(output, "  - %s\n", value); err != nil {
			return err
		}
	}
	return nil
}

func rootStorageMetrics(preview orchestrator.Preview) (int64, int64, int64, bool) {
	for _, filesystem := range preview.Storage.Filesystems {
		if filesystem.MountPoint == "/" {
			return filesystem.TotalBytes, filesystem.FreeBytes, filesystem.AvailableBytes, true
		}
	}
	if len(preview.Storage.Filesystems) > 0 {
		filesystem := preview.Storage.Filesystems[0]
		return filesystem.TotalBytes, filesystem.FreeBytes, filesystem.AvailableBytes, true
	}
	return 0, 0, 0, false
}

func availability(available bool) string {
	if available {
		return "available"
	}
	return "unavailable"
}

func valueOrUnknown(value string) string {
	if strings.TrimSpace(value) == "" {
		return "unknown"
	}
	return value
}

func knownBytes(value int64, known bool) string {
	if !known {
		return "unknown"
	}
	return fmt.Sprintf("%d", value)
}
