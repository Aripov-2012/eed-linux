package resources

import (
	"fmt"
	"sort"
	"strings"

	"eed/internal/components"
	"eed/internal/detection"
	"eed/internal/planner"
)

// CPUReport captures CPU-related resource checks.
type CPUReport struct {
	LogicalCores          int    `json:"logical_cores"`
	PhysicalCores         int    `json:"physical_cores"`
	RequiredLogicalCores  int    `json:"required_logical_cores,omitempty"`
	RequiredPhysicalCores int    `json:"required_physical_cores,omitempty"`
	Status                string `json:"status"`
}

// RAMReport captures memory-related resource checks.
type RAMReport struct {
	TotalBytes     int64  `json:"total_bytes"`
	AvailableBytes int64  `json:"available_bytes"`
	RequiredBytes  int64  `json:"required_bytes,omitempty"`
	Status         string `json:"status"`
}

// StorageReport captures storage-related resource checks.
type StorageReport struct {
	AvailableBytes             int64            `json:"available_bytes"`
	TotalPackageFootprintBytes int64            `json:"total_package_footprint_bytes,omitempty"`
	TotalPackageDownloadBytes  int64            `json:"total_package_download_bytes,omitempty"`
	EstimatedDownloadBytes     int64            `json:"estimated_download_bytes,omitempty"`
	EstimatedInstallBytes      int64            `json:"estimated_install_bytes,omitempty"`
	AdditionalDownloadBytes    int64            `json:"additional_download_bytes,omitempty"`
	AdditionalInstallBytes     int64            `json:"additional_install_bytes,omitempty"`
	TotalPackageFootprintKnown bool             `json:"total_package_footprint_known"`
	TotalPackageDownloadKnown  bool             `json:"total_package_download_known"`
	AdditionalDownloadKnown    bool             `json:"additional_download_known"`
	AdditionalInstallKnown     bool             `json:"additional_install_known"`
	EstimatedDownloadKnown     bool             `json:"estimated_download_known"`
	EstimatedInstallKnown      bool             `json:"estimated_install_known"`
	EstimateComplete           bool             `json:"estimate_complete"`
	EstimateConfidence         string           `json:"estimate_confidence"`
	EstimateSources            []EstimateSource `json:"estimate_sources,omitempty"`
	EstimateNotes              []string         `json:"estimate_notes,omitempty"`
	Status                     string           `json:"status"`
}

type EstimateSource struct {
	Package      string `json:"package,omitempty"`
	Architecture string `json:"architecture,omitempty"`
	Version      string `json:"version,omitempty"`
	Source       string `json:"source"`
	AsOf         string `json:"as_of,omitempty"`
	Confidence   string `json:"confidence,omitempty"`
}

// SafetyReserve defines the current reserve policy, if any.
type SafetyReserve struct {
	Policy string `json:"policy"`
	Bytes  int64  `json:"bytes,omitempty"`
}

// Report summarizes resource analysis.
type Report struct {
	Status        string        `json:"status"`
	CPU           CPUReport     `json:"cpu"`
	RAM           RAMReport     `json:"ram"`
	Storage       StorageReport `json:"storage"`
	SafetyReserve SafetyReserve `json:"safety_reserve"`
	Warnings      []string      `json:"warnings,omitempty"`
	Errors        []string      `json:"errors,omitempty"`
}

// Analyze inspects the detected machine and selected components for resource viability.
func Analyze(report detection.Report, registry *components.Registry, plan planner.Plan) Report {
	reportOut := Report{
		Status:        "unknown",
		CPU:           CPUReport{LogicalCores: report.Hardware.CPU.LogicalCores, PhysicalCores: report.Hardware.CPU.PhysicalCores, Status: "unknown"},
		RAM:           RAMReport{TotalBytes: report.Hardware.Memory.TotalBytes, AvailableBytes: report.Hardware.Memory.AvailableBytes, Status: "unknown"},
		Storage:       StorageReport{AvailableBytes: sumAvailableStorage(report.System.Storage), EstimateComplete: false, Status: "unknown"},
		SafetyReserve: SafetyReserve{Policy: "unspecified", Bytes: 0},
		Warnings:      []string{},
		Errors:        []string{},
	}

	selected := make([]string, 0, len(plan.Components))
	for _, component := range plan.Components {
		if component.ID != "" {
			selected = append(selected, component.ID)
		}
	}
	if registry != nil {
		selected = append(selected, selectedFromRegistry(registry, selected)...)
	}
	selected = uniqueStrings(selected)

	cpuThresholds := maxCPURequirements(registry, selected)
	reportOut.CPU.RequiredLogicalCores = cpuThresholds[0]
	reportOut.CPU.RequiredPhysicalCores = cpuThresholds[1]
	if report.Hardware.CPU.LogicalCores > 0 && reportOut.CPU.RequiredLogicalCores > 0 {
		if report.Hardware.CPU.LogicalCores >= reportOut.CPU.RequiredLogicalCores {
			reportOut.CPU.Status = "sufficient"
		} else {
			reportOut.CPU.Status = "insufficient"
			reportOut.Errors = append(reportOut.Errors, "Logical CPU core requirement not met.")
		}
	} else if report.Hardware.CPU.LogicalCores == 0 {
		reportOut.CPU.Status = "unknown"
		reportOut.Warnings = append(reportOut.Warnings, "CPU core count is unknown.")
	}

	ramThreshold := maxRAMRequirement(registry, selected)
	reportOut.RAM.RequiredBytes = ramThreshold
	if report.Hardware.Memory.TotalBytes > 0 && ramThreshold > 0 {
		if report.Hardware.Memory.TotalBytes >= ramThreshold {
			reportOut.RAM.Status = "sufficient"
		} else {
			reportOut.RAM.Status = "insufficient"
			reportOut.Errors = append(reportOut.Errors, "Total installed RAM is below the required threshold.")
		}
	} else if ramThreshold == 0 {
		reportOut.RAM.Status = "sufficient"
	} else {
		reportOut.RAM.Status = "unknown"
		reportOut.Warnings = append(reportOut.Warnings, "Total RAM is unavailable; memory threshold could not be verified.")
	}

	estimateArchitecture := report.System.Architecture
	if estimateArchitecture == "" {
		estimateArchitecture = report.Hardware.CPU.Architecture
	}
	var storageEstimate, downloadEstimate int64
	var estimateComplete bool
	var estimateWarnings []string
	osID := strings.ToLower(strings.TrimSpace(report.System.OS.ID))
	if osID == "" {
		osID = strings.ToLower(strings.TrimSpace(report.System.OS.Name))
	}
	isArch := osID == "arch" || osID == "arch linux"
	if isArch {
		storageEstimate, downloadEstimate, estimateComplete, estimateWarnings = estimateStorage(registry, selected, estimateArchitecture)
	} else if osID == "" || osID == "unknown" {
		estimateWarnings = []string{"Host distribution is unknown; Arch-specific manifest package estimates were not applied."}
	} else {
		estimateWarnings = []string{"Manifest package estimates are Arch-specific and were not applied to this distribution; use native cached package metadata instead."}
	}
	reportOut.Storage.EstimatedInstallBytes = storageEstimate
	reportOut.Storage.EstimatedDownloadBytes = downloadEstimate
	reportOut.Storage.EstimateComplete = estimateComplete
	reportOut.Storage.EstimatedDownloadKnown = estimateComplete
	reportOut.Storage.EstimatedInstallKnown = estimateComplete
	reportOut.Storage.EstimateNotes = estimateWarnings
	if isArch {
		reportOut.Storage.EstimateSources = estimateSources(registry, selected, estimateArchitecture)
	}
	reportOut.Storage.EstimateConfidence = "unavailable"
	if estimateComplete {
		reportOut.Storage.EstimateConfidence = "approximate"
	} else if storageEstimate > 0 || downloadEstimate > 0 {
		reportOut.Storage.EstimateConfidence = "partial"
	}
	if estimateWarnings != nil {
		reportOut.Warnings = append(reportOut.Warnings, estimateWarnings...)
	}
	if reportOut.Storage.AvailableBytes > 0 {
		if storageEstimate > 0 && !estimateComplete {
			reportOut.Storage.Status = "incomplete"
		} else if storageEstimate > 0 && reportOut.Storage.AvailableBytes >= storageEstimate {
			reportOut.Storage.Status = "sufficient"
		} else if storageEstimate > 0 {
			reportOut.Storage.Status = "insufficient"
			reportOut.Errors = append(reportOut.Errors, "Available storage is below the estimated installation requirement.")
		} else {
			reportOut.Storage.Status = "estimate_unavailable"
			reportOut.Warnings = append(reportOut.Warnings, "Filesystem capacity is known, but installation feasibility cannot be assessed because selected components have no size estimates.")
		}
	} else {
		reportOut.Storage.Status = "unknown"
		reportOut.Warnings = append(reportOut.Warnings, "Storage availability is unavailable; installation feasibility remains unknown.")
	}
	if len(reportOut.Errors) > 0 {
		reportOut.Status = "insufficient"
	} else if len(reportOut.Warnings) > 0 || !reportOut.Storage.EstimateComplete {
		reportOut.Status = "limited"
	} else {
		reportOut.Status = "sufficient"
	}
	if reportOut.Storage.EstimateComplete == false {
		reportOut.Status = "incomplete"
	}
	return reportOut
}

func selectedFromRegistry(registry *components.Registry, ids []string) []string {
	if registry == nil {
		return nil
	}
	res := make([]string, 0)
	seen := map[string]bool{}
	for _, id := range ids {
		seen[id] = true
	}
	for _, id := range registry.Order {
		if seen[id] {
			res = append(res, id)
		}
	}
	return res
}

func uniqueStrings(values []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(values))
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	return out
}

func maxCPURequirements(registry *components.Registry, ids []string) [2]int {
	maxLogical, maxPhysical := 0, 0
	for _, id := range ids {
		if registry == nil {
			continue
		}
		manifest, ok := registry.Get(id)
		if !ok {
			continue
		}
		if manifest.Requirements.MinimumLogicalCores > maxLogical {
			maxLogical = manifest.Requirements.MinimumLogicalCores
		}
		if manifest.Requirements.MinimumPhysicalCores > maxPhysical {
			maxPhysical = manifest.Requirements.MinimumPhysicalCores
		}
	}
	return [2]int{maxLogical, maxPhysical}
}

func maxRAMRequirement(registry *components.Registry, ids []string) int64 {
	maxVal := int64(0)
	for _, id := range ids {
		if registry == nil {
			continue
		}
		manifest, ok := registry.Get(id)
		if !ok {
			continue
		}
		if manifest.Requirements.MinimumTotalRAMBytes > maxVal {
			maxVal = manifest.Requirements.MinimumTotalRAMBytes
		}
	}
	return maxVal
}

func estimateStorage(registry *components.Registry, ids []string, architecture string) (int64, int64, bool, []string) {
	if registry == nil {
		return 0, 0, false, []string{"Component registry is unavailable; package size estimates cannot be calculated."}
	}
	seenComponents := map[string]bool{}
	packageMetadata := map[string]components.PackageSizeEstimate{}
	packageConflicts := map[string]bool{}
	packageOwners := map[string]map[string]bool{}
	directPackages := map[string]bool{}
	missingByComponent := map[string]map[string]bool{}
	var installedTotal, downloadTotal int64
	complete := true
	warnings := make([]string, 0)
	for _, id := range ids {
		manifest, ok := registry.Get(id)
		if !ok {
			complete = false
			warnings = append(warnings, "Component "+id+" has no manifest; its installation size cannot be estimated.")
			continue
		}
		if seenComponents[id] {
			continue
		}
		seenComponents[id] = true
		if len(manifest.Packages) == 0 {
			complete = false
			note := manifest.Resources.EstimateNotes
			if note == "" {
				note = "the component declares no packages"
			}
			warnings = append(warnings, "Component "+id+" size estimate unavailable: "+strings.TrimRight(note, ". ")+".")
			continue
		}
		for _, pkg := range manifest.Packages {
			pkg = normalizePackageName(pkg)
			directPackages[pkg] = true
			addPackageOwner(packageOwners, pkg, id)
		}
		if len(manifest.Resources.PackageEstimates) == 0 && manifest.Resources.EstimatedInstalledBytes > 0 {
			installedTotal += manifest.Resources.EstimatedInstalledBytes
			downloadTotal += manifest.Resources.EstimatedDownloadBytes
			if manifest.Resources.EstimatedDownloadBytes == 0 {
				complete = false
				warnings = append(warnings, "Component "+id+" has no download size estimate.")
			}
			continue
		}
		for _, estimate := range manifest.Resources.PackageEstimates {
			if !estimateMatchesArchitecture(estimate.Architecture, architecture) {
				continue
			}
			packageName := normalizePackageName(estimate.Package)
			if existing, exists := packageMetadata[packageName]; exists {
				if existing.EstimatedInstalledBytes != estimate.EstimatedInstalledBytes ||
					existing.EstimatedDownloadBytes != estimate.EstimatedDownloadBytes {
					packageConflicts[packageName] = true
				}
				continue
			}
			packageMetadata[packageName] = estimate
		}
	}
	directPackageList := make([]string, 0, len(directPackages))
	for pkg := range directPackages {
		directPackageList = append(directPackageList, pkg)
	}
	sort.Strings(directPackageList)
	queue := directPackageList
	seenPackages := map[string]bool{}
	for len(queue) > 0 {
		pkg := queue[0]
		queue = queue[1:]
		if seenPackages[pkg] {
			continue
		}
		seenPackages[pkg] = true
		estimate, ok := packageMetadata[pkg]
		if !ok {
			complete = false
			owners := packageOwners[pkg]
			if len(owners) == 0 {
				addMissingPackage(missingByComponent, "selected components", pkg)
			} else {
				for owner := range owners {
					addMissingPackage(missingByComponent, owner, pkg)
				}
			}
			continue
		}
		if packageConflicts[pkg] {
			complete = false
			warnings = append(warnings, "Conflicting size metadata for package "+pkg+"; its estimate may be stale.")
		}
		installedTotal += estimate.EstimatedInstalledBytes
		downloadTotal += estimate.EstimatedDownloadBytes
		for _, dependency := range estimate.Dependencies {
			dependency = normalizePackageName(dependency)
			for owner := range packageOwners[pkg] {
				addPackageOwner(packageOwners, dependency, owner)
			}
			queue = append(queue, dependency)
		}
	}
	componentIDs := make([]string, 0, len(missingByComponent))
	for componentID := range missingByComponent {
		componentIDs = append(componentIDs, componentID)
	}
	sort.Strings(componentIDs)
	for _, componentID := range componentIDs {
		missingNames := make([]string, 0, len(missingByComponent[componentID]))
		for pkg := range missingByComponent[componentID] {
			missingNames = append(missingNames, pkg)
		}
		sort.Strings(missingNames)
		sample := missingNames
		if len(sample) > 5 {
			sample = sample[:5]
		}
		details := strings.Join(sample, ", ")
		if len(missingNames) > len(sample) {
			details += fmt.Sprintf(", and %d more", len(missingNames)-len(sample))
		}
		warnings = append(warnings, fmt.Sprintf("Component %s has incomplete size metadata for %d package or runtime dependency entries (%s).", componentID, len(missingNames), details))
	}
	if len(ids) == 0 {
		complete = false
		warnings = append(warnings, "No selected components were available for package size estimation.")
	}
	return installedTotal, downloadTotal, complete, warnings
}

func addPackageOwner(owners map[string]map[string]bool, packageName, owner string) {
	if owners[packageName] == nil {
		owners[packageName] = map[string]bool{}
	}
	owners[packageName][owner] = true
}

func addMissingPackage(missing map[string]map[string]bool, componentID, packageName string) {
	if missing[componentID] == nil {
		missing[componentID] = map[string]bool{}
	}
	missing[componentID][packageName] = true
}

func estimateMatchesArchitecture(estimateArchitecture, targetArchitecture string) bool {
	if estimateArchitecture == "" {
		return true
	}
	if targetArchitecture == "" {
		return false
	}
	return normalizeArchitecture(estimateArchitecture) == normalizeArchitecture(targetArchitecture)
}

func normalizeArchitecture(architecture string) string {
	switch strings.ToLower(strings.TrimSpace(architecture)) {
	case "amd64", "x86_64":
		return "x86_64"
	case "arm64", "aarch64":
		return "aarch64"
	default:
		return strings.ToLower(strings.TrimSpace(architecture))
	}
}

func normalizePackageName(dependency string) string {
	dependency = strings.TrimSpace(dependency)
	if separator := strings.IndexAny(dependency, "<>="); separator >= 0 {
		dependency = dependency[:separator]
	}
	return strings.TrimSpace(dependency)
}

func estimateSources(registry *components.Registry, ids []string, architecture string) []EstimateSource {
	if registry == nil {
		return nil
	}
	sources := map[string]EstimateSource{}
	seen := map[string]bool{}
	for _, id := range ids {
		if seen[id] {
			continue
		}
		seen[id] = true
		manifest, ok := registry.Get(id)
		if !ok {
			continue
		}
		for _, estimate := range manifest.Resources.PackageEstimates {
			if estimate.Source == "" || !estimateMatchesArchitecture(estimate.Architecture, architecture) {
				continue
			}
			key := strings.Join([]string{
				estimate.Package, estimate.Architecture, estimate.Version, estimate.Source, estimate.AsOf,
			}, "|")
			sources[key] = EstimateSource{
				Package: estimate.Package, Architecture: estimate.Architecture, Version: estimate.Version,
				Source: estimate.Source, AsOf: estimate.AsOf, Confidence: estimate.Confidence,
			}
		}
		if manifest.Resources.EstimateSource != "" {
			key := manifest.Resources.EstimateSource + "|" + manifest.Resources.EstimateAsOf
			sources[key] = EstimateSource{
				Source: manifest.Resources.EstimateSource, AsOf: manifest.Resources.EstimateAsOf,
				Confidence: manifest.Resources.EstimateConfidence,
			}
		}
	}
	keys := make([]string, 0, len(sources))
	for key := range sources {
		keys = append(keys, key)
	}
	sort.Strings(keys)
	result := make([]EstimateSource, 0, len(keys))
	for _, key := range keys {
		result = append(result, sources[key])
	}
	return result
}

func sumAvailableStorage(storage []detection.MountInfo) int64 {
	for _, item := range storage {
		if item.MountPoint == "/" {
			return item.AvailableBytes
		}
	}
	for _, item := range storage {
		return item.AvailableBytes
	}
	return 0
}

func sortedKeys(values []string) []string {
	out := append([]string(nil), values...)
	sort.Strings(out)
	return out
}
