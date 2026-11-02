package compatibility

import (
	"strings"

	"eed/internal/components"
	"eed/internal/detection"
)

func architectureChecks(report detection.Report, registry *components.Registry, selected []string) []Check {
	checks := make([]Check, 0)
	if registry == nil {
		return checks
	}
	detectedArch := normalizeArchitecture(report.System.Architecture)
	if detectedArch == "" {
		detectedArch = normalizeArchitecture(report.System.Kernel.Architecture)
	}
	if detectedArch == "" {
		return []Check{{Name: "architecture", Status: "unknown", Message: "Detected architecture is unavailable."}}
	}
	for _, id := range selected {
		manifest, ok := registry.Get(id)
		if !ok {
			continue
		}
		allowed := manifest.Compatibility.SupportedArchitectures
		if len(allowed) == 0 {
			allowed = manifest.SupportedArchitectures
		}
		if len(allowed) == 0 {
			continue
		}
		matched := false
		for _, arch := range allowed {
			if normalizeArchitecture(arch) == detectedArch {
				matched = true
				break
			}
		}
		if !matched {
			checks = append(checks, Check{Name: id, Status: "incompatible", Message: "Component " + id + " does not support the detected architecture " + detectedArch})
		}
	}
	return checks
}

func normalizeArchitecture(value string) string {
	switch strings.ToLower(strings.TrimSpace(value)) {
	case "amd64", "x86_64":
		return "x86_64"
	case "arm64", "aarch64":
		return "aarch64"
	default:
		return strings.ToLower(strings.TrimSpace(value))
	}
}
