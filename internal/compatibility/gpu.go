package compatibility

import (
	"strings"

	"eed/internal/components"
	"eed/internal/detection"
)

func gpuChecks(report detection.Report, registry *components.Registry, selected []string) []Check {
	checks := make([]Check, 0)
	if registry == nil {
		return checks
	}
	for _, id := range selected {
		manifest, ok := registry.Get(id)
		if !ok {
			continue
		}
		vendors := manifest.Compatibility.RequiredGPUVendor
		checkName := "required_gpu_vendor"
		if len(vendors) == 0 {
			vendors = manifest.Compatibility.SupportedGPUVendors
			checkName = "supported_gpu_vendors"
		}
		if len(vendors) == 0 {
			continue
		}
		if len(report.Hardware.GPUs) == 0 {
			checks = append(checks, Check{Name: id + "." + checkName, Status: "unknown", Message: "GPU vendor compatibility could not be verified because no GPUs were detected."})
			continue
		}
		matched := false
		knownVendor := false
		for _, gpu := range report.Hardware.GPUs {
			detectedVendor := normalizeGPUVendor(gpu.Vendor)
			if detectedVendor == "" || detectedVendor == "unknown" {
				detectedVendor = normalizeGPUVendor(gpu.VendorID)
			}
			if detectedVendor == "" || detectedVendor == "unknown" {
				continue
			}
			knownVendor = true
			for _, supportedVendor := range vendors {
				if normalizeGPUVendor(supportedVendor) == detectedVendor {
					matched = true
				}
			}
		}
		if matched {
			continue
		}
		if !knownVendor {
			checks = append(checks, Check{Name: id + "." + checkName, Status: "unknown", Message: "GPU vendor compatibility could not be verified because detected GPU vendor information is unavailable."})
			continue
		}
		if checkName == "required_gpu_vendor" {
			checks = append(checks, Check{Name: id + "." + checkName, Status: "incompatible", Message: "No detected GPU matches the required vendor set: " + strings.Join(vendors, ", ") + "."})
		} else {
			checks = append(checks, Check{Name: id + "." + checkName, Status: "incompatible", Message: "No detected GPU is in the component's supported vendor set: " + strings.Join(vendors, ", ") + "."})
		}
	}
	return checks
}

func normalizeGPUVendor(vendor string) string {
	switch strings.ToLower(strings.TrimSpace(vendor)) {
	case "0x8086", "8086", "intel":
		return "intel"
	case "0x1002", "1002", "0x1022", "1022", "amd", "advanced micro devices":
		return "amd"
	case "0x10de", "10de", "nvidia":
		return "nvidia"
	default:
		return strings.ToLower(strings.TrimSpace(vendor))
	}
}
