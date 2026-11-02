package assessment

import (
	"eed/internal/compatibility"
	"eed/internal/components"
	"eed/internal/config"
	"eed/internal/detection"
	"eed/internal/planner"
	"eed/internal/resources"
)

// Assessment is the unified result of compatibility and resource evaluation.
type Assessment struct {
	SchemaVersion int                  `json:"schema_version"`
	OverallStatus string               `json:"overall_status"`
	Compatibility compatibility.Report `json:"compatibility"`
	Resources     resources.Report     `json:"resources"`
	Warnings      []string             `json:"warnings,omitempty"`
	Errors        []string             `json:"errors,omitempty"`
}

// Evaluate combines compatibility and resource analysis into a single read-only assessment.
func Evaluate(cfg config.Config, plan planner.Plan, registry *components.Registry, report detection.Report) (Assessment, error) {
	compat, err := compatibility.Assess(cfg, plan, registry, report)
	if err != nil {
		return Assessment{}, err
	}
	res := resources.Analyze(report, registry, plan)

	warnings := append([]string{}, compat.Warnings...)
	warnings = append(warnings, res.Warnings...)
	errors := append([]string{}, compat.Errors...)
	errors = append(errors, res.Errors...)

	overall := "ready"
	switch {
	case len(errors) > 0:
		overall = "blocked"
	case compat.Status == "diagnostic_only":
		overall = "diagnostic_only"
	case compat.Status == "compatible_with_warnings" || res.Status == "limited" || res.Status == "incomplete":
		overall = "needs_review"
	case len(warnings) > 0:
		overall = "needs_review"
	}

	return Assessment{
		SchemaVersion: 1,
		OverallStatus: overall,
		Compatibility: compat,
		Resources:     res,
		Warnings:      warnings,
		Errors:        errors,
	}, nil
}
