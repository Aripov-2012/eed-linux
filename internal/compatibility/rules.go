package compatibility

import (
	"strings"

	"eed/internal/components"
	"eed/internal/detection"
)

func osCompatibilityCheck(report detection.Report) []Check {
	checks := make([]Check, 0, 1)
	osName := normalizeOSName(report.System.OS.ID)
	osPretty := normalizeOSName(report.System.OS.Name)
	if osName == "arch" || strings.Contains(osPretty, "arch") {
		checks = append(checks, Check{Name: "os", Status: "ok", Message: "Host OS is Arch Linux."})
		return checks
	}
	if osName == "" && osPretty == "" {
		checks = append(checks, Check{Name: "os", Status: "unknown", Message: "Operating system could not be determined."})
		return checks
	}
	checks = append(checks, Check{Name: "os", Status: "warning", Message: "Host OS is not Arch Linux; the plan is diagnostic only."})
	return checks
}

func requirementChecks(report detection.Report, registry *components.Registry, selected []string) []Check {
	checks := make([]Check, 0)
	if registry == nil {
		return checks
	}
	for _, id := range selected {
		manifest, ok := registry.Get(id)
		if !ok {
			continue
		}
		if len(manifest.Compatibility.RequiredInit) > 0 {
			if !containsAny(strings.ToLower(report.System.InitSystem), manifest.Compatibility.RequiredInit) {
				checks = append(checks, Check{Name: id, Status: "warning", Message: "Component requires init system " + strings.Join(manifest.Compatibility.RequiredInit, ", ") + ", but this was not detected."})
			}
		}
		if len(manifest.Compatibility.RequiredSession) > 0 {
			if !containsAny(strings.ToLower(report.System.SessionType), manifest.Compatibility.RequiredSession) {
				checks = append(checks, Check{Name: id, Status: "warning", Message: "Component requires session type " + strings.Join(manifest.Compatibility.RequiredSession, ", ") + ", but this was not detected."})
			}
		}
	}
	return checks
}

func containsAny(haystack string, values []string) bool {
	for _, value := range values {
		if strings.Contains(haystack, strings.ToLower(strings.TrimSpace(value))) {
			return true
		}
	}
	return false
}
