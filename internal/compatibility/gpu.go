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
		if manifest.Compatibility.RequiresHardwareAcceleration {
			graphics := report.Hardware.Graphics
			switch {
			case graphics.HardwareAcceleratedRendering:
			case graphics.SoftwareRenderingFallback:
				checks = append(checks, Check{
					Name: id + ".hardware_acceleration", Status: "warning",
					Message: "Hardware acceleration is unavailable; the detected OpenGL software renderer (" + graphics.OpenGLRenderer + ") may limit this component.",
				})
			case graphics.OpenGLRenderer == "":
				message := "Hardware acceleration could not be verified because no OpenGL renderer was detected."
				if graphics.RendererError != "" {
					message += " Renderer probe: " + graphics.RendererError
				}
				checks = append(checks, Check{
					Name: id + ".hardware_acceleration", Status: "unknown",
					Message: message,
				})
			}
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
		virtualDetected := false
		for _, gpu := range report.Hardware.GPUs {
			if gpu.Virtual {
				virtualDetected = true
				continue
			}
			detectedVendor := normalizeGPUVendor(gpu.Vendor)
			if detectedVendor == "" || detectedVendor == "unknown" {
				detectedVendor = normalizeGPUVendor(gpuVendorFromPCI(gpu.VendorID))
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
		if virtualDetected {
			checks = append(checks, Check{
				Name: id + "." + checkName, Status: "warning",
				Message: "A virtual GPU was detected; virtual hardware is allowed for diagnostics and package planning, but vendor-specific compatibility is not verified.",
			})
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
	case "intel":
		return "intel"
	case "amd", "advanced micro devices":
		return "amd"
	case "nvidia":
		return "nvidia"
	default:
		return strings.ToLower(strings.TrimSpace(vendor))
	}
}

func gpuVendorFromPCI(vendorID string) string {
	switch strings.TrimPrefix(strings.ToLower(strings.TrimSpace(vendorID)), "0x") {
	case "8086":
		return "intel"
	case "1002", "1022":
		return "amd"
	case "10de":
		return "nvidia"
	default:
		return "unknown"
	}
}
