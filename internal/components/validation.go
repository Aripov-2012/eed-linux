package components

import (
	"fmt"
	"strings"
)

func validateManifest(m Manifest) error {
	if m.SchemaVersion == 0 {
		return fmt.Errorf("schema_version is required")
	}
	if strings.TrimSpace(m.ID) == "" {
		return fmt.Errorf("id is required")
	}
	if strings.TrimSpace(m.Name) == "" {
		return fmt.Errorf("name is required")
	}
	if strings.TrimSpace(m.Category) == "" {
		return fmt.Errorf("category is required")
	}
	if len(m.SupportedOS) == 0 {
		return fmt.Errorf("supported_os must contain at least one supported target")
	}
	for _, osName := range m.SupportedOS {
		if strings.TrimSpace(osName) == "" {
			return fmt.Errorf("supported_os contains an empty value")
		}
	}
	for _, arch := range m.SupportedArchitectures {
		if strings.TrimSpace(arch) == "" {
			return fmt.Errorf("supported_architectures contains an empty value")
		}
	}
	for _, dep := range m.Dependencies {
		dep = strings.TrimSpace(dep)
		if dep == "" {
			return fmt.Errorf("dependencies contains an empty value")
		}
	}
	for _, conflict := range m.Conflicts {
		conflict = strings.TrimSpace(conflict)
		if conflict == "" {
			return fmt.Errorf("conflicts contains an empty value")
		}
	}
	for _, pkg := range m.Packages {
		pkg = strings.TrimSpace(pkg)
		if pkg == "" {
			return fmt.Errorf("packages contains an empty value")
		}
	}
	if m.Resources.EstimatedInstalledBytes < 0 || m.Resources.EstimatedDownloadBytes < 0 {
		return fmt.Errorf("resource estimates cannot be negative")
	}
	if m.Resources.EstimatedInstalledBytes > 0 || m.Resources.EstimatedDownloadBytes > 0 {
		if m.Resources.EstimatedInstalledBytes == 0 || m.Resources.EstimatedDownloadBytes == 0 {
			return fmt.Errorf("component size estimate must include download and installed sizes")
		}
		if strings.TrimSpace(m.Resources.EstimateSource) == "" || strings.TrimSpace(m.Resources.EstimateConfidence) == "" {
			return fmt.Errorf("component size estimate must identify its metadata source and confidence")
		}
	}
	for _, estimate := range m.Resources.PackageEstimates {
		if strings.TrimSpace(estimate.Package) == "" {
			return fmt.Errorf("package_estimates contains an empty package")
		}
		if estimate.EstimatedInstalledBytes < 0 || estimate.EstimatedDownloadBytes < 0 {
			return fmt.Errorf("package size estimates cannot be negative")
		}
		if estimate.EstimatedInstalledBytes == 0 || estimate.EstimatedDownloadBytes == 0 {
			return fmt.Errorf("package size estimate for %q must include download and installed sizes", estimate.Package)
		}
		if strings.TrimSpace(estimate.Source) == "" {
			return fmt.Errorf("package size estimate for %q must identify its metadata source", estimate.Package)
		}
		if strings.TrimSpace(estimate.Confidence) == "" {
			return fmt.Errorf("package size estimate for %q must identify its confidence", estimate.Package)
		}
		if strings.TrimSpace(estimate.Version) == "" || strings.TrimSpace(estimate.AsOf) == "" {
			return fmt.Errorf("package size estimate for %q must identify its package version and metadata date", estimate.Package)
		}
		for _, dependency := range estimate.Dependencies {
			if strings.TrimSpace(dependency) == "" {
				return fmt.Errorf("package size estimate for %q contains an empty dependency", estimate.Package)
			}
		}
	}
	return nil
}
