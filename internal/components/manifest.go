package components

import "time"

// CompatibilityRequirements is an optional compatibility section on a manifest.
type CompatibilityRequirements struct {
	SupportedOS                  []string `toml:"supported_os"`
	SupportedArchitectures       []string `toml:"supported_architectures"`
	RequiredInit                 []string `toml:"required_init"`
	RequiredSession              []string `toml:"required_session"`
	RequiredKernelFeatures       []string `toml:"required_kernel_features"`
	RequiredGPUVendor            []string `toml:"required_gpu_vendor"`
	SupportedGPUVendors          []string `toml:"supported_gpu_vendors"`
	RequiredGPUClass             []string `toml:"required_gpu_class"`
	RequiresHardwareAcceleration bool     `toml:"requires_hardware_acceleration"`
}

// Requirements define runtime and resource constraints for a component.
type Requirements struct {
	MinimumLogicalCores      int   `toml:"minimum_logical_cores"`
	MinimumPhysicalCores     int   `toml:"minimum_physical_cores"`
	MinimumTotalRAMBytes     int64 `toml:"minimum_total_ram_bytes"`
	RecommendedTotalRAMBytes int64 `toml:"recommended_total_ram_bytes"`
	MinimumFreeRAMBytes      int64 `toml:"minimum_free_ram_bytes"`
	MinimumStorageBytes      int64 `toml:"minimum_storage_bytes"`
	RecommendedStorageBytes  int64 `toml:"recommended_storage_bytes"`
}

// ResourceEstimate captures optional installation sizing metadata.
type ResourceEstimate struct {
	EstimatedInstalledBytes int64                 `toml:"estimated_installed_bytes"`
	EstimatedDownloadBytes  int64                 `toml:"estimated_download_bytes"`
	EstimateConfidence      string                `toml:"estimate_confidence"`
	EstimateSource          string                `toml:"estimate_source"`
	EstimateAsOf            string                `toml:"estimate_as_of"`
	EstimateNotes           string                `toml:"estimate_notes"`
	PackageEstimates        []PackageSizeEstimate `toml:"package_estimates"`
}

// PackageSizeEstimate records package-manager metadata for one declared package.
type PackageSizeEstimate struct {
	Package                 string   `toml:"package"`
	Architecture            string   `toml:"architecture"`
	Version                 string   `toml:"version"`
	EstimatedInstalledBytes int64    `toml:"estimated_installed_bytes"`
	EstimatedDownloadBytes  int64    `toml:"estimated_download_bytes"`
	Dependencies            []string `toml:"dependencies"`
	Confidence              string   `toml:"confidence"`
	Source                  string   `toml:"source"`
	AsOf                    string   `toml:"as_of"`
}

// Manifest describes a component available in the registry.
type Manifest struct {
	SchemaVersion          int                       `toml:"schema_version"`
	ID                     string                    `toml:"id"`
	Name                   string                    `toml:"name"`
	Description            string                    `toml:"description"`
	Category               string                    `toml:"category"`
	SupportedOS            []string                  `toml:"supported_os"`
	SupportedArchitectures []string                  `toml:"supported_architectures"`
	Dependencies           []string                  `toml:"dependencies"`
	Conflicts              []string                  `toml:"conflicts"`
	Packages               []string                  `toml:"packages"`
	PackageVariants        map[string][]string       `toml:"package_variants"`
	DefaultEnabled         bool                      `toml:"default_enabled"`
	Compatibility          CompatibilityRequirements `toml:"compatibility"`
	Requirements           Requirements              `toml:"requirements"`
	Resources              ResourceEstimate          `toml:"resources"`
	Metadata               map[string]string         `toml:"metadata"`
	LoadedAt               time.Time                 `toml:"-"`
}
