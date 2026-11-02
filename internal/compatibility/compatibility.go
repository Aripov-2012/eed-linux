package compatibility

import (
	"strings"

	"eed/internal/components"
	"eed/internal/config"
	"eed/internal/customization"
	"eed/internal/detection"
	"eed/internal/planner"
)

// Check records a single compatibility assessment result.
type Check struct {
	Name    string `json:"name"`
	Status  string `json:"status"`
	Message string `json:"message,omitempty"`
}

// Report summarizes compatibility checks for a plan.
type Report struct {
	Status   string   `json:"status"`
	Checks   []Check  `json:"checks,omitempty"`
	Warnings []string `json:"warnings,omitempty"`
	Errors   []string `json:"errors,omitempty"`
}

// Assess runs read-only compatibility checks for the current plan and environment.
func Assess(cfg config.Config, plan planner.Plan, registry *components.Registry, report detection.Report) (Report, error) {
	state, err := customization.NormalizeFromConfig(cfg)
	if err != nil {
		return Report{}, err
	}
	selected := customization.SelectedComponents(state)
	checks := make([]Check, 0)
	warnings := make([]string, 0)
	errors := make([]string, 0)

	checks = append(checks, osCompatibilityCheck(report)...)
	checks = append(checks, architectureChecks(report, registry, selected)...)
	checks = append(checks, desktopCompatibilityChecks(registry, selected)...)
	checks = append(checks, gpuChecks(report, registry, selected)...)
	checks = append(checks, requirementChecks(report, registry, selected)...)

	for _, c := range checks {
		switch c.Status {
		case "warning":
			warnings = append(warnings, c.Message)
		case "incompatible":
			errors = append(errors, c.Message)
		case "unknown":
			warnings = append(warnings, c.Message)
		}
	}

	status := "compatible"
	switch {
	case len(errors) > 0:
		status = "incompatible"
	case len(warnings) > 0:
		status = "compatible_with_warnings"
	}
	if len(errors) == 0 && hasUnsupportedHostWarning(checks) {
		status = "diagnostic_only"
	}
	if plan.Status == "blocked" {
		if len(plan.Errors) > 0 || len(errors) > 0 {
			status = "incompatible"
		} else if hasUnsupportedHostWarning(checks) {
			status = "diagnostic_only"
		}
	}
	if report.System.OS.ID == "" && report.System.OS.Name == "" {
		status = "unknown"
	}

	return Report{
		Status:   status,
		Checks:   checks,
		Warnings: warnings,
		Errors:   errors,
	}, nil
}

func hasUnsupportedHostWarning(checks []Check) bool {
	for _, check := range checks {
		if check.Name == "os" && check.Status == "warning" {
			return true
		}
	}
	return false
}

func addCheck(list []Check, name, status, message string) []Check {
	if name == "" {
		return list
	}
	return append(list, Check{Name: name, Status: status, Message: message})
}

func normalizeOSName(value string) string {
	return strings.ToLower(strings.TrimSpace(value))
}
