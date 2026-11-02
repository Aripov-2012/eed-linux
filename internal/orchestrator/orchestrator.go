package orchestrator

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	builtin "eed"
	"eed/internal/assessment"
	"eed/internal/components"
	"eed/internal/config"
	"eed/internal/detection"
	"eed/internal/packageinfo"
	"eed/internal/planner"
	"eed/internal/portalconfig"
	"eed/internal/snapshot"
	"eed/internal/snapshot/snapper"
	"eed/internal/storage"
)

type StageStatus string

const (
	StageCompleted   StageStatus = "completed"
	StageSkipped     StageStatus = "skipped"
	StageBlocked     StageStatus = "blocked"
	StageUnavailable StageStatus = "unavailable"
)

type Stage struct {
	ID        string      `json:"id"`
	Status    StageStatus `json:"status"`
	StartedAt time.Time   `json:"started_at,omitempty"`
	EndedAt   time.Time   `json:"ended_at,omitempty"`
	Message   string      `json:"message,omitempty"`
}

type Preview struct {
	SchemaVersion   string                         `json:"schema_version"`
	GeneratedAt     time.Time                      `json:"generated_at"`
	Mode            string                         `json:"mode"`
	Profile         string                         `json:"profile"`
	DetectedDesktop string                         `json:"detected_desktop"`
	TargetDesktop   string                         `json:"target_desktop"`
	Selected        []string                       `json:"selected_components"`
	PackageManagers []detection.PackageManagerInfo `json:"package_managers,omitempty"`
	PackageReport   packageinfo.Report             `json:"package_report"`
	PortalPlan      *portalconfig.Plan             `json:"portal_plan,omitempty"`
	DetectedGPUs    []detection.GPUInfo            `json:"detected_gpus,omitempty"`
	DetectedCPU     detection.CPUInfo              `json:"detected_cpu"`
	DetectedMemory  detection.MemoryInfo           `json:"detected_memory"`
	DetectedStorage []detection.MountInfo          `json:"detected_storage,omitempty"`
	Virtualization  string                         `json:"virtualization,omitempty"`
	BlockDevices    []detection.BlockDevice        `json:"block_devices,omitempty"`
	Plan            planner.Plan                   `json:"plan"`
	Assessment      assessment.Assessment          `json:"assessment"`
	Storage         storage.Report                 `json:"storage"`
	Snapshot        snapshot.Capability            `json:"snapshot_capability"`
	SnapshotFacts   SnapshotFacts                  `json:"snapshot_facts"`
	Stages          []Stage                        `json:"stages"`
	Changes         ChangeSummary                  `json:"proposed_changes"`
	Warnings        []string                       `json:"warnings,omitempty"`
	Blockers        []string                       `json:"blockers,omitempty"`
	Diagnostics     []Diagnostic                   `json:"diagnostics,omitempty"`
	Summary         ReportSummary                  `json:"summary"`
}

type Diagnostic struct {
	Severity           string `json:"severity"`
	Subsystem          string `json:"subsystem"`
	Operation          string `json:"operation"`
	Component          string `json:"component,omitempty"`
	Reason             string `json:"reason"`
	BlocksInstallation bool   `json:"blocks_installation"`
}

type SnapshotFacts struct {
	FilesystemDetected           string   `json:"filesystem_detected"`
	FilesystemSupported          bool     `json:"filesystem_supported"`
	SnapshotToolInstalled        bool     `json:"snapshot_tool_installed"`
	BackendConfigured            bool     `json:"backend_configured"`
	SnapshotCreationSupported    bool     `json:"snapshot_creation_supported"`
	SnapshotCreationVerified     bool     `json:"snapshot_creation_verified"`
	FullRollbackVerified         bool     `json:"full_rollback_verified"`
	LVMThinDetected              bool     `json:"lvm_thin_detected"`
	LVMLogicalVolumeDetected     bool     `json:"lvm_logical_volume_detected"`
	SnapshotModel                string   `json:"snapshot_model"`
	BtrfsLayoutInspected         bool     `json:"btrfs_layout_inspected"`
	BtrfsSeparateMounts          []string `json:"btrfs_separate_mounts,omitempty"`
	ExternalVMSnapshotSuggested  bool     `json:"external_vm_snapshot_suggested"`
	EEDManagedFilesystemSnapshot bool     `json:"eed_managed_filesystem_snapshot"`
	Status                       string   `json:"status"`
	Reason                       string   `json:"reason"`
}

type ReportSummary struct {
	Detected           []string `json:"detected,omitempty"`
	CouldNotDetermine  []string `json:"could_not_determine,omitempty"`
	Incompatible       []string `json:"incompatible,omitempty"`
	NotImplemented     []string `json:"not_implemented,omitempty"`
	FutureInstallation []string `json:"future_installation,omitempty"`
}

type ChangeSummary struct {
	PackageCandidates            []string `json:"package_candidates,omitempty"`
	TransactionPackageCandidates []string `json:"transaction_package_candidates,omitempty"`
	InstalledPackagesKnown       bool     `json:"installed_packages_known"`
	ConfigurationFilesKnown      bool     `json:"configuration_files_known"`
	ServiceChangesKnown          bool     `json:"service_changes_known"`
	FullSystemUpgrade            bool     `json:"full_system_upgrade"`
	SnapshotCreationPlanned      bool     `json:"snapshot_creation_planned"`
}

type Dependencies struct {
	ConfigPath           string
	ManifestPath         string
	LoadConfig           func(string) (config.Config, error)
	Detect               func(context.Context) (detection.Report, error)
	LoadRegistry         func(string) (*components.Registry, error)
	AnalyzeStore         func() (storage.Report, error)
	ProbeSnapshot        func(context.Context) snapshot.Capability
	InspectPackages      func(context.Context, []string) (packageinfo.Report, error)
	InspectPackagesForOS func(context.Context, string, []string) (packageinfo.Report, error)
	PlanPortals          func(string, map[string]bool) (portalconfig.Plan, error)
}

func DefaultDependencies() Dependencies {
	return Dependencies{
		LoadConfig: config.LoadConfig,
		Detect:     detection.Detect,
		LoadRegistry: func(dataDir string) (*components.Registry, error) {
			return components.LoadDiscovered(dataDir, builtin.BuiltinManifests())
		},
		AnalyzeStore: storage.Analyze,
		ProbeSnapshot: func(ctx context.Context) snapshot.Capability {
			probeCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
			defer cancel()
			return snapper.NewBackend().Probe(probeCtx)
		},
	}
}

func BuildPreview(ctx context.Context, deps Dependencies) (Preview, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	defaults := DefaultDependencies()
	if deps.LoadConfig == nil {
		deps.LoadConfig = defaults.LoadConfig
	}
	if deps.Detect == nil {
		deps.Detect = defaults.Detect
	}
	if deps.LoadRegistry == nil {
		deps.LoadRegistry = defaults.LoadRegistry
	}
	if deps.AnalyzeStore == nil {
		deps.AnalyzeStore = defaults.AnalyzeStore
	}
	if deps.ProbeSnapshot == nil {
		deps.ProbeSnapshot = defaults.ProbeSnapshot
	}

	preview := Preview{
		SchemaVersion: "1",
		GeneratedAt:   time.Now().UTC(),
		Mode:          "read_only_preview",
		Stages:        []Stage{},
		Changes: ChangeSummary{
			InstalledPackagesKnown:  false,
			ConfigurationFilesKnown: false,
			ServiceChangesKnown:     false,
			FullSystemUpgrade:       false,
			SnapshotCreationPlanned: false,
		},
		Warnings:    []string{},
		Blockers:    []string{},
		Diagnostics: []Diagnostic{},
	}

	var cfg config.Config
	if err := runStageOnly(ctx, &preview, "load_config", "load and validate user configuration", func() error {
		var loadErr error
		cfg, loadErr = deps.LoadConfig(deps.ConfigPath)
		return loadErr
	}); err != nil {
		return preview, err
	}
	preview.Profile = cfg.Profile.Name

	var host detection.Report
	if err := runStageOnly(ctx, &preview, "detect_system", "collect read-only system facts", func() error {
		var detectErr error
		host, detectErr = deps.Detect(ctx)
		host.Warnings = uniqueWarnings(host.Warnings)
		return detectErr
	}); err != nil {
		return preview, err
	}
	preview.PackageManagers = append([]detection.PackageManagerInfo{}, host.Software.PackageManagers...)
	preview.DetectedGPUs = append([]detection.GPUInfo{}, host.Hardware.GPUs...)
	preview.DetectedDesktop = host.System.Desktop
	preview.DetectedCPU = host.Hardware.CPU
	preview.DetectedMemory = host.Hardware.Memory
	preview.DetectedStorage = append([]detection.MountInfo(nil), host.System.Storage...)
	preview.Virtualization = host.System.Virtualization
	preview.BlockDevices = append([]detection.BlockDevice(nil), host.System.BlockDevices...)

	registryPath := deps.ManifestPath
	var registry *components.Registry
	if err := runStageOnly(ctx, &preview, "load_components", "load and validate component manifests", func() error {
		var loadErr error
		registry, loadErr = deps.LoadRegistry(registryPath)
		return loadErr
	}); err != nil {
		return preview, err
	}

	var installPlan planner.Plan
	if err := runStageOnly(ctx, &preview, "create_plan", "resolve selected components and dependencies", func() error {
		var planErr error
		installPlan, planErr = planner.BuildPlan(cfg, registry, host)
		installPlan.Warnings = uniqueWarnings(installPlan.Warnings)
		return planErr
	}); err != nil {
		return preview, err
	}
	preview.Plan = installPlan
	preview.TargetDesktop = targetDesktop(cfg.Desktop.Environment)
	preview.Selected = make([]string, 0, len(installPlan.Components))
	for _, component := range installPlan.Components {
		if component.Selected {
			preview.Selected = append(preview.Selected, component.ID)
		}
	}
	preview.Changes.PackageCandidates = append([]string{}, installPlan.Packages...)
	preview.PackageReport = packageinfo.UnavailableReport(installPlan.Packages, "unknown", "installed package inventory and cached repository metadata have not been inspected")
	hostOS := strings.ToLower(strings.TrimSpace(installPlan.HostOS))
	var inspectPackages func(context.Context, []string) (packageinfo.Report, error)
	if hostOS == "arch" || hostOS == "arch linux" {
		inspectPackages = deps.InspectPackages
		if inspectPackages == nil && deps.InspectPackagesForOS != nil {
			inspectPackages = func(ctx context.Context, packages []string) (packageinfo.Report, error) {
				return deps.InspectPackagesForOS(ctx, hostOS, packages)
			}
		}
	} else if hostOS == "ubuntu" || hostOS == "debian" {
		if deps.InspectPackagesForOS != nil {
			inspectPackages = func(ctx context.Context, packages []string) (packageinfo.Report, error) {
				return deps.InspectPackagesForOS(ctx, hostOS, packages)
			}
		}
	}
	if inspectPackages != nil {
		if err := runNonFatalStage(ctx, &preview, "inspect_packages", "read installed package inventory and cached repository metadata", "packages", func() error {
			var inspectErr error
			preview.PackageReport, inspectErr = inspectPackages(ctx, installPlan.Packages)
			if inspectErr != nil {
				failedReport := packageinfo.UnavailableReport(installPlan.Packages, "unavailable", "package inventory and cached repository metadata could not be inspected")
				failedReport.Errors = append(failedReport.Errors, inspectErr.Error())
				preview.PackageReport = failedReport
				return inspectErr
			}
			if preview.PackageReport.Status == "unavailable" {
				if len(preview.PackageReport.Errors) > 0 {
					return errors.New(preview.PackageReport.Errors[0])
				}
				if len(preview.PackageReport.Warnings) > 0 {
					return errors.New(preview.PackageReport.Warnings[0])
				}
				return errors.New("package inventory is unavailable")
			}
			return nil
		}); err != nil {
			return preview, err
		}
	} else if hostOS == "arch" || hostOS == "arch linux" {
		preview.PackageReport = packageinfo.UnavailableReport(installPlan.Packages, "unavailable", "Arch package inspector is not configured")
	} else if hostOS == "ubuntu" || hostOS == "debian" {
		preview.PackageReport = packageinfo.UnavailableReport(installPlan.Packages, "unavailable", "APT package inspector is not configured")
	} else {
		preview.PackageReport = packageinfo.UnavailableReport(installPlan.Packages, "not_applicable_host", "No read-only package inventory adapter is configured for this distribution")
	}
	preview.Plan.PackageStatuses = append([]packageinfo.Candidate(nil), preview.PackageReport.Candidates...)
	preview.Warnings = appendVisibleWarnings(preview.Warnings, preview.PackageReport.Warnings...)
	for _, warning := range preview.PackageReport.Warnings {
		appendDiagnostic(&preview, Diagnostic{
			Severity: "warning", Subsystem: "packages", Operation: "inspect installed packages and cached repository metadata", Reason: warning,
		})
	}
	for _, packageError := range preview.PackageReport.Errors {
		appendDiagnostic(&preview, Diagnostic{
			Severity: "error", Subsystem: "packages", Operation: "inspect installed packages and cached repository metadata", Reason: packageError,
		})
	}
	_, portalManifestAvailable := registry.Get("system.portals")
	if portalManifestAvailable && includesComponent(installPlan.Components, "system.portals") {
		var installed map[string]bool
		if preview.PackageReport.Status == "available" {
			installed = make(map[string]bool, len(preview.PackageReport.Candidates))
			for _, candidate := range preview.PackageReport.Candidates {
				if candidate.InstalledVersion != "" {
					installed[candidate.Name] = true
				}
			}
		}
		if err := runNonFatalStage(ctx, &preview, "plan_portals", "inspect existing portal configuration and propose desktop-specific selection", "portals", func() error {
			if deps.PlanPortals == nil {
				return errors.New("portal policy adapter is not configured")
			}
			plan, planErr := deps.PlanPortals(cfg.Desktop.Environment, installed)
			if planErr == nil {
				preview.PortalPlan = &plan
			}
			return planErr
		}); err != nil {
			return preview, err
		}
		if preview.PortalPlan != nil {
			preview.Warnings = appendVisibleWarnings(preview.Warnings, preview.PortalPlan.Limitations...)
		}
	}
	preview.Warnings = appendVisibleWarnings(preview.Warnings, host.Warnings...)

	if err := runStageOnly(ctx, &preview, "assess", "evaluate compatibility and resource requirements", func() error {
		var assessErr error
		preview.Assessment, assessErr = assessment.Evaluate(cfg, installPlan, registry, host)
		preview.Assessment.Warnings = uniqueWarnings(preview.Assessment.Warnings)
		return assessErr
	}); err != nil {
		return preview, err
	}
	if preview.PackageReport.InventoryKnown {
		preview.Changes.InstalledPackagesKnown = true
	}
	if preview.PackageReport.Status == "available" {
		preview.Changes.TransactionPackageCandidates = make([]string, 0)
		for _, candidate := range preview.PackageReport.Candidates {
			if candidate.State == packageinfo.StateNotInstalled || candidate.State == packageinfo.StateVersionMismatch {
				preview.Changes.TransactionPackageCandidates = append(preview.Changes.TransactionPackageCandidates, candidate.Name)
			}
		}
		preview.Assessment.Resources.Storage.TotalPackageFootprintBytes = preview.PackageReport.TotalInstalledBytes
		preview.Assessment.Resources.Storage.TotalPackageDownloadBytes = preview.PackageReport.TotalDownloadBytes
		preview.Assessment.Resources.Storage.TotalPackageFootprintKnown = preview.PackageReport.TotalInstalledKnown
		preview.Assessment.Resources.Storage.TotalPackageDownloadKnown = preview.PackageReport.TotalDownloadKnown
		preview.Assessment.Resources.Storage.AdditionalDownloadBytes = preview.PackageReport.AdditionalDownloadBytes
		preview.Assessment.Resources.Storage.AdditionalInstallBytes = preview.PackageReport.AdditionalInstallBytes
		preview.Assessment.Resources.Storage.AdditionalDownloadKnown = preview.PackageReport.AdditionalDownloadKnown
		preview.Assessment.Resources.Storage.AdditionalInstallKnown = preview.PackageReport.AdditionalInstallKnown
		preview.Assessment.Resources.Storage.EstimatedDownloadKnown = preview.PackageReport.TotalDownloadKnown
		preview.Assessment.Resources.Storage.EstimatedInstallKnown = preview.PackageReport.TotalInstalledKnown
		if len(preview.PackageReport.ResolvedPackages) > 0 {
			preview.Assessment.Resources.Storage.EstimatedDownloadBytes = preview.PackageReport.TotalDownloadBytes
			preview.Assessment.Resources.Storage.EstimatedInstallBytes = preview.PackageReport.TotalInstalledBytes
		}
		preview.Assessment.Resources.Storage.EstimateComplete = preview.PackageReport.EstimateComplete
		preview.Assessment.Resources.Storage.EstimateConfidence = "partial"
		if preview.PackageReport.EstimateComplete {
			preview.Assessment.Resources.Storage.EstimateConfidence = "approximate"
		}
		preview.Assessment.Resources.Storage.Status = "incomplete"
		if preview.PackageReport.EstimateComplete {
			preview.Assessment.Resources.Storage.Status = "informational"
		}
	}

	if err := ctx.Err(); err != nil {
		return preview, err
	}
	storageStarted := time.Now().UTC()
	var storageErr error
	preview.Storage, storageErr = deps.AnalyzeStore()
	preview.Storage.Warnings = uniqueWarnings(preview.Storage.Warnings)
	storageStage := Stage{ID: "analyze_storage", StartedAt: storageStarted, EndedAt: time.Now().UTC(), Message: "inspect mounted filesystems and capacity", Status: StageCompleted}
	if storageErr != nil {
		storageStage.Status = StageUnavailable
		storageStage.Message = storageErr.Error()
		preview.Warnings = append(preview.Warnings, "storage analysis unavailable: "+storageErr.Error())
	}
	preview.Stages = append(preview.Stages, storageStage)

	if err := runStageOnly(ctx, &preview, "snapshot_capability", "probe snapshot capability without creating a snapshot", func() error {
		preview.Snapshot = deps.ProbeSnapshot(ctx)
		return nil
	}); err != nil {
		return preview, err
	}

	preview.Warnings = appendVisibleWarnings(preview.Warnings, installPlan.Warnings...)
	preview.Warnings = appendVisibleWarnings(preview.Warnings, preview.Assessment.Warnings...)
	preview.Warnings = append(preview.Warnings, preview.Storage.Warnings...)
	rootFilesystem := "unknown"
	for _, filesystem := range preview.Storage.Filesystems {
		if filesystem.MountPoint == "/" {
			rootFilesystem = filesystem.Filesystem
			break
		}
	}
	preview.SnapshotFacts = assessSnapshotFactsWithStorage(rootFilesystem, preview.Snapshot, preview.Storage, host.System.Virtualization)
	preview.Warnings = append(preview.Warnings, "snapshot protection is not confirmed: "+preview.SnapshotFacts.Reason)
	managerBlockers := []string{
		"package installation is unavailable: the approved Pacman transaction adapter is not wired into an installation workflow",
		"configuration deployment is unavailable: the configuration manager is not implemented",
		"service configuration is unavailable: the service manager is not implemented",
		"verification and persistence do not yet have installation ownership/journal APIs",
	}
	preview.Blockers = append(preview.Blockers, managerBlockers...)
	preview.Blockers = append(preview.Blockers, installPlan.Errors...)
	preview.Blockers = append(preview.Blockers, preview.Assessment.Errors...)
	if !installPlan.Installable {
		preview.Blockers = append(preview.Blockers, "installation plan is not installable on this host")
	}
	for _, warning := range host.Warnings {
		subsystem, operation := classifyWarning(warning)
		appendDiagnostic(&preview, Diagnostic{Severity: warningSeverity(warning), Subsystem: subsystem, Operation: operation, Component: warningComponent(warning), Reason: warning})
	}
	for _, warning := range installPlan.Warnings {
		appendDiagnostic(&preview, Diagnostic{Severity: warningSeverity(warning), Subsystem: "planner", Operation: "resolve component plan", Component: warningComponent(warning), Reason: warning})
	}
	for _, warning := range preview.Assessment.Warnings {
		appendDiagnostic(&preview, Diagnostic{Severity: warningSeverity(warning), Subsystem: "assessment", Operation: "evaluate compatibility and resources", Component: warningComponent(warning), Reason: warning})
	}
	for _, warning := range preview.Storage.Warnings {
		appendDiagnostic(&preview, Diagnostic{Severity: "warning", Subsystem: "storage", Operation: "inspect filesystem metadata", Reason: warning})
	}
	if storageErr != nil {
		appendDiagnostic(&preview, Diagnostic{
			Severity:  "error",
			Subsystem: "storage", Operation: "inspect mounted filesystems and capacity",
			Reason: storageErr.Error(), BlocksInstallation: true,
		})
	}
	appendDiagnostic(&preview, Diagnostic{
		Severity: "warning", Subsystem: "snapshot", Operation: "inspect snapshot backend",
		Reason: preview.SnapshotFacts.Reason,
	})
	for _, planError := range installPlan.Errors {
		appendDiagnostic(&preview, Diagnostic{
			Severity:  "error",
			Subsystem: "planner", Operation: "validate selected components", Reason: planError, BlocksInstallation: true,
		})
	}
	for _, assessmentError := range preview.Assessment.Errors {
		appendDiagnostic(&preview, Diagnostic{
			Severity:  "error",
			Subsystem: "assessment", Operation: "evaluate compatibility and resources", Reason: assessmentError, BlocksInstallation: true,
		})
	}
	preview.Warnings = uniqueWarnings(preview.Warnings)
	for _, blocker := range managerBlockers {
		preview.Diagnostics = append(preview.Diagnostics, Diagnostic{
			Severity: "error", Subsystem: "installation", Operation: "validate installation capability", Reason: blocker, BlocksInstallation: true,
		})
	}
	if !installPlan.Installable {
		appendDiagnostic(&preview, Diagnostic{
			Severity: "info", Subsystem: "planner", Operation: "validate host installation target",
			Reason: "installation plan is not installable on this host", BlocksInstallation: true,
		})
	}
	preview.Summary = buildReportSummary(preview, managerBlockers)
	return preview, nil
}

func assessSnapshotFacts(filesystem string, capability snapshot.Capability) SnapshotFacts {
	return assessSnapshotFactsWithStorage(filesystem, capability, storage.Report{}, "")
}

func assessSnapshotFactsWithStorage(filesystem string, capability snapshot.Capability, storageReport storage.Report, virtualization string) SnapshotFacts {
	facts := SnapshotFacts{
		FilesystemDetected:    filesystem,
		FilesystemSupported:   strings.EqualFold(filesystem, "btrfs"),
		SnapshotToolInstalled: capability.ToolInstalled,
		BackendConfigured:     capability.ConfigurationExists,
		SnapshotCreationSupported: strings.EqualFold(filesystem, "btrfs") &&
			capability.ToolInstalled && capability.ConfigurationExists && capability.CreationSupported,
		FullRollbackVerified: false,
		BtrfsLayoutInspected: storageReport.BtrfsLayoutInspected,
		BtrfsSeparateMounts:  append([]string(nil), storageReport.BtrfsSeparateMounts...),
		ExternalVMSnapshotSuggested: virtualization != "" && !strings.EqualFold(virtualization, "none") &&
			!strings.EqualFold(virtualization, "unknown"),
		EEDManagedFilesystemSnapshot: false,
	}
	for _, root := range storageReport.Filesystems {
		if root.MountPoint == "/" {
			facts.LVMLogicalVolumeDetected = root.LVMLogicalVolume
			facts.LVMThinDetected = root.LVMThin
			break
		}
	}
	switch {
	case facts.LVMThinDetected:
		facts.SnapshotModel = "lvm_thin_unmanaged"
	case strings.EqualFold(filesystem, "btrfs"):
		facts.SnapshotModel = "btrfs_snapper_unverified"
	case strings.EqualFold(filesystem, "ext4") && facts.LVMLogicalVolumeDetected:
		facts.SnapshotModel = "lvm_layout_not_verified"
	case strings.EqualFold(filesystem, "ext4"):
		facts.SnapshotModel = "limited_or_config_specific"
	default:
		facts.SnapshotModel = "unsupported"
	}
	if facts.LVMThinDetected {
		facts.FilesystemSupported = false
	}
	switch {
	case filesystem == "" || strings.EqualFold(filesystem, "unknown"):
		facts.Status = "filesystem_unknown"
		facts.Reason = "The target filesystem could not be determined; snapshot support is unknown."
	case facts.LVMThinDetected:
		facts.Status = "lvm_thin_unmanaged"
		facts.Reason = "An LVM thin volume was detected, but EED has no configured LVM snapshot backend; no EED-managed protection is claimed."
	case strings.EqualFold(filesystem, "ext4") && facts.LVMLogicalVolumeDetected:
		facts.Status = "lvm_snapshot_unverified"
		facts.Reason = "An ext4 filesystem on an LVM logical volume was detected, but the snapshot layout and backend type are not verified; no protection is claimed."
	case !facts.FilesystemSupported:
		facts.Status = "unsupported_filesystem"
		if strings.EqualFold(filesystem, "ext4") {
			facts.Reason = "ext4 snapshot support depends on a suitable LVM layout and backend; EED has not verified one and claims no snapshot protection."
		} else {
			facts.Reason = fmt.Sprintf("%s does not provide the configured Btrfs/Snapper snapshot protection.", filesystem)
		}
	case !capability.ToolInstalled:
		facts.Status = "snapshot_tool_missing"
		facts.Reason = "The Btrfs filesystem is supported, but the Snapper tool is not installed."
	case !capability.ConfigurationExists:
		facts.Status = "backend_not_configured"
		facts.Reason = "Btrfs and Snapper are available, but a Snapper backend configuration was not found."
	case !capability.CreationSupported:
		facts.Status = "snapshot_creation_unsupported"
		facts.Reason = "The configured snapshot backend does not report snapshot creation support."
	case !facts.FullRollbackVerified:
		facts.Status = "full_rollback_unverified"
		facts.Reason = "The backend is configured, but snapshot creation and full rollback have not been verified; no protection is claimed."
		if !storageReport.BtrfsLayoutInspected {
			facts.Reason += " The Btrfs subvolume layout could not be inspected."
		} else if len(facts.BtrfsSeparateMounts) > 0 {
			facts.Reason += " Separate Btrfs mountpoints may not be covered: " + strings.Join(facts.BtrfsSeparateMounts, ", ") + "."
		}
	default:
		facts.Status = "ready"
		facts.Reason = "Snapshot creation and full rollback have been verified."
	}
	return facts
}

func buildReportSummary(preview Preview, notImplemented []string) ReportSummary {
	summary := ReportSummary{
		Detected:           []string{},
		CouldNotDetermine:  []string{},
		Incompatible:       []string{},
		NotImplemented:     append([]string{}, notImplemented...),
		FutureInstallation: []string{},
	}
	hostOS := preview.Plan.HostOS
	if hostOS != "" {
		summary.Detected = append(summary.Detected, "Host operating system: "+hostOS)
	} else {
		summary.CouldNotDetermine = append(summary.CouldNotDetermine, "Host operating system.")
	}
	for _, manager := range preview.PackageManagers {
		if manager.Native && manager.Available {
			summary.Detected = append(summary.Detected, "Native package manager: "+manager.Name)
		}
	}
	if len(preview.DetectedGPUs) == 0 {
		summary.CouldNotDetermine = append(summary.CouldNotDetermine, "No GPU was identified by available read-only probes.")
	} else {
		for _, gpu := range preview.DetectedGPUs {
			name := strings.TrimSpace(gpu.Model)
			if name == "" {
				name = strings.TrimSpace(gpu.Vendor)
			} else if gpu.Vendor != "" && !strings.Contains(strings.ToLower(name), strings.ToLower(gpu.Vendor)) {
				name = strings.TrimSpace(gpu.Vendor + " " + name)
			}
			summary.Detected = append(summary.Detected, "GPU: "+name)
		}
	}
	rootFound := false
	for _, filesystem := range preview.Storage.Filesystems {
		if filesystem.MountPoint == "/" {
			rootFound = true
			summary.Detected = append(summary.Detected, fmt.Sprintf("Root filesystem: %s; %d bytes available to the current user", filesystem.Filesystem, filesystem.AvailableBytes))
			break
		}
	}
	if !rootFound {
		summary.CouldNotDetermine = append(summary.CouldNotDetermine, "Target root filesystem capacity.")
	}
	if !preview.Assessment.Resources.Storage.EstimateComplete {
		if preview.Assessment.Resources.Storage.EstimatedInstallBytes > 0 ||
			preview.Assessment.Resources.Storage.EstimatedDownloadBytes > 0 {
			summary.CouldNotDetermine = append(summary.CouldNotDetermine, "Complete dependency and configuration-footprint sizes; displayed package totals are partial.")
		} else {
			summary.CouldNotDetermine = append(summary.CouldNotDetermine, "Download and installed-size estimates; unavailable Arch repository metadata was not guessed.")
		}
	}
	if !preview.Changes.InstalledPackagesKnown {
		summary.CouldNotDetermine = append(summary.CouldNotDetermine, "Which package candidates are already installed.")
	}
	for _, warning := range preview.PackageReport.Warnings {
		summary.CouldNotDetermine = append(summary.CouldNotDetermine, warning)
	}
	if !preview.Changes.ConfigurationFilesKnown {
		summary.CouldNotDetermine = append(summary.CouldNotDetermine, "Future configuration-file changes.")
	}
	if !preview.Changes.ServiceChangesKnown {
		summary.CouldNotDetermine = append(summary.CouldNotDetermine, "Future service changes.")
	}
	if !preview.SnapshotFacts.FilesystemSupported && preview.SnapshotFacts.FilesystemDetected != "" &&
		!strings.EqualFold(preview.SnapshotFacts.FilesystemDetected, "unknown") {
		summary.Incompatible = append(summary.Incompatible, fmt.Sprintf(
			"%s does not provide the configured Btrfs/Snapper snapshot protection.", preview.SnapshotFacts.FilesystemDetected))
	}
	if !preview.SnapshotFacts.FullRollbackVerified {
		summary.CouldNotDetermine = append(summary.CouldNotDetermine, "Full rollback has not been verified; snapshot protection is not claimed.")
	}
	for _, group := range preview.Storage.SkippedMounts {
		if group.Category == "inaccessible_optional" {
			summary.CouldNotDetermine = append(summary.CouldNotDetermine, fmt.Sprintf("Capacity for %d inaccessible optional mountpoints.", group.Count))
		}
	}
	if !preview.Plan.Installable && hostOS != "" &&
		!strings.EqualFold(hostOS, "arch") && !strings.EqualFold(hostOS, "arch linux") {
		summary.Incompatible = append(summary.Incompatible, fmt.Sprintf("Host %s is not a valid Arch installation target; this plan is diagnostic only.", hostOSOrUnknown(hostOS)))
	}
	summary.Incompatible = append(summary.Incompatible, preview.Plan.Errors...)
	summary.Incompatible = append(summary.Incompatible, preview.Assessment.Errors...)
	if len(preview.Changes.PackageCandidates) > 0 {
		summary.FutureInstallation = append(summary.FutureInstallation, "A future installer may consider the listed package candidates; no package transaction was built or executed.")
	} else {
		summary.FutureInstallation = append(summary.FutureInstallation, "No package candidates are declared; selected components are preserved without substitution.")
	}
	summary.FutureInstallation = append(summary.FutureInstallation, "Test Drive performed no package, configuration, service, or snapshot changes.")
	return summary
}

func hostOSOrUnknown(hostOS string) string {
	if strings.TrimSpace(hostOS) == "" {
		return "unknown"
	}
	return hostOS
}

func appendVisibleWarnings(destination []string, warnings ...string) []string {
	for _, warning := range warnings {
		if !isExpectedUnsupportedOS(warning) {
			destination = append(destination, warning)
		}
	}
	return destination
}

func warningSeverity(warning string) string {
	if isExpectedUnsupportedOS(warning) {
		return "info"
	}
	return "warning"
}

func isExpectedUnsupportedOS(warning string) bool {
	lower := strings.ToLower(warning)
	return strings.Contains(lower, "not arch linux") ||
		strings.Contains(lower, "not installable on this host") ||
		strings.Contains(lower, "diagnostic only")
}

func warningComponent(warning string) string {
	lower := strings.ToLower(warning)
	if !strings.HasPrefix(lower, "component ") {
		return ""
	}
	fields := strings.Fields(warning)
	if len(fields) < 2 {
		return ""
	}
	return strings.Trim(fields[1], ":.,")
}

func classifyWarning(warning string) (string, string) {
	lower := strings.ToLower(warning)
	switch {
	case strings.HasPrefix(warning, "storage analysis unavailable:") || strings.Contains(warning, "mountinfo"):
		return "storage", "inspect filesystem metadata"
	case strings.Contains(lower, "snapper") || strings.Contains(lower, "snapshot"):
		return "snapshot", "inspect snapshot backend"
	case strings.Contains(lower, "package manager"):
		return "software detection", "detect installed package manager"
	case strings.Contains(lower, "gpu"):
		return "hardware detection", "detect PCI graphics devices"
	case strings.Contains(lower, "read /proc") || strings.Contains(lower, "read /sys"):
		return "hardware detection", "read kernel and hardware metadata"
	case strings.Contains(lower, "permission denied"), strings.Contains(lower, "operation not permitted"):
		return "system detection", "read host resource"
	case strings.Contains(lower, "component"):
		return "planner", "resolve selected components"
	case strings.Contains(lower, "resource"), strings.Contains(lower, "compatibility"):
		return "assessment", "evaluate host compatibility and resources"
	default:
		return "system detection", "collect optional host information"
	}
}

func appendDiagnostic(preview *Preview, diagnostic Diagnostic) {
	for _, existing := range preview.Diagnostics {
		if existing.Severity == diagnostic.Severity &&
			existing.Subsystem == diagnostic.Subsystem &&
			existing.Operation == diagnostic.Operation &&
			existing.Component == diagnostic.Component &&
			existing.Reason == diagnostic.Reason &&
			existing.BlocksInstallation == diagnostic.BlocksInstallation {
			return
		}
	}
	preview.Diagnostics = append(preview.Diagnostics, diagnostic)
}

func uniqueWarnings(warnings []string) []string {
	seen := make(map[string]struct{}, len(warnings))
	unique := make([]string, 0, len(warnings))
	for _, warning := range warnings {
		if _, exists := seen[warning]; exists {
			continue
		}
		seen[warning] = struct{}{}
		unique = append(unique, warning)
	}
	return unique
}

func runStageOnly(ctx context.Context, preview *Preview, id, message string, action func() error) error {
	started := time.Now().UTC()
	if err := ctx.Err(); err != nil {
		preview.Stages = append(preview.Stages, Stage{ID: id, Status: StageBlocked, StartedAt: started, EndedAt: time.Now().UTC(), Message: "cancelled before stage execution"})
		return err
	}
	err := action()
	stage := Stage{ID: id, StartedAt: started, EndedAt: time.Now().UTC(), Message: message, Status: StageCompleted}
	if err != nil {
		stage.Status = StageBlocked
		stage.Message = err.Error()
	}
	preview.Stages = append(preview.Stages, stage)
	return err
}

func runNonFatalStage(ctx context.Context, preview *Preview, id, message, subsystem string, action func() error) error {
	started := time.Now().UTC()
	if err := ctx.Err(); err != nil {
		preview.Stages = append(preview.Stages, Stage{ID: id, Status: StageBlocked, StartedAt: started, EndedAt: time.Now().UTC(), Message: "cancelled before stage execution"})
		return err
	}
	err := action()
	stage := Stage{ID: id, StartedAt: started, EndedAt: time.Now().UTC(), Message: message, Status: StageCompleted}
	if err != nil {
		if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
			stage.Status = StageBlocked
			stage.Message = err.Error()
			preview.Stages = append(preview.Stages, stage)
			return err
		}
		stage.Status = StageUnavailable
		stage.Message = err.Error()
		preview.Warnings = append(preview.Warnings, message+" unavailable: "+err.Error())
		appendDiagnostic(preview, Diagnostic{
			Severity: "warning", Subsystem: subsystem, Operation: message, Reason: err.Error(),
		})
	}
	preview.Stages = append(preview.Stages, stage)
	return nil
}

func includesComponent(components []planner.ComponentPlan, id string) bool {
	for _, component := range components {
		if component.ID == id && component.Selected {
			return true
		}
	}
	return false
}

func targetDesktop(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	switch value {
	case "", "none", "desktop.none":
		return "none"
	case "hyprland", "desktop.hyprland":
		return "desktop.hyprland"
	case "gnome", "desktop.gnome":
		return "desktop.gnome"
	case "kde", "plasma", "desktop.kde":
		return "desktop.kde"
	case "xfce", "desktop.xfce":
		return "desktop.xfce"
	case "existing", "desktop.existing":
		return "existing"
	default:
		return value
	}
}

func (p Preview) Validate() error {
	if p.SchemaVersion != "1" {
		return fmt.Errorf("unsupported preview schema version %q", p.SchemaVersion)
	}
	if p.Mode != "read_only_preview" {
		return fmt.Errorf("unexpected preview mode %q", p.Mode)
	}
	return nil
}
