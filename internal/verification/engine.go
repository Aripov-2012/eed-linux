package verification

import (
	"context"
	"fmt"
	"strings"
	"time"
)

type Engine struct {
	Registry     *Registry
	CheckTimeout time.Duration
}

func (e Engine) Run(ctx context.Context, input Context, componentFilter string) (Report, error) {
	if e.Registry == nil {
		return Report{}, fmt.Errorf("verification registry is required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	if input.Runner == nil {
		input.Runner = ExecRunner{}
	}
	timeout := e.CheckTimeout
	if timeout <= 0 {
		timeout = 10 * time.Second
	}

	report := Report{
		SchemaVersion:        "1",
		CollectedAt:          time.Now().UTC(),
		OverallStatus:        "completed",
		Components:           []ComponentResult{},
		RecoveryAvailability: "unknown: recovery managers and ownership manifests are not implemented",
	}
	componentFilter = strings.TrimSpace(componentFilter)
	foundComponent := componentFilter == ""
	componentIndex := map[string]int{}
	for _, check := range e.Registry.Checks() {
		if componentFilter != "" && check.ComponentID != componentFilter {
			continue
		}
		foundComponent = true
		if _, exists := componentIndex[check.ComponentID]; !exists {
			componentIndex[check.ComponentID] = len(report.Components)
			report.Components = append(report.Components, ComponentResult{
				ComponentID: check.ComponentID,
				Status:      StatusSkipped,
				Checks:      []CheckResult{},
			})
		}

		var result CheckResult
		if !input.IsSelected(check.ComponentID) {
			result = CheckResult{Status: StatusSkipped, Message: "component is not selected in the active configuration"}
		} else {
			checkCtx, cancel := context.WithTimeout(ctx, timeout)
			result = check.Run(checkCtx, input)
			cancel()
			if result.Status == "" {
				result.Status = StatusUnknown
				result.Message = "check returned no status"
			}
		}
		result.ID = check.ID
		result.Name = check.Name
		result.Description = check.Description
		result.Severity = check.Severity
		result.Required = check.Required
		index := componentIndex[check.ComponentID]
		report.Components[index].Checks = append(report.Components[index].Checks, result)
	}
	if !foundComponent {
		return Report{}, fmt.Errorf("no verification checks registered for component %q", componentFilter)
	}

	for i := range report.Components {
		report.Components[i].Status = aggregateStatus(report.Components[i].Checks)
		for _, check := range report.Components[i].Checks {
			switch check.Status {
			case StatusPass:
				report.Summary.Passed++
			case StatusWarning:
				report.Summary.Warnings++
			case StatusFail:
				report.Summary.Failed++
			case StatusUnknown:
				report.Summary.Unknown++
			case StatusSkipped:
				report.Summary.Skipped++
			}
		}
	}
	switch {
	case report.Summary.Failed > 0:
		report.OverallStatus = "completed_with_failures"
	case report.Summary.Warnings > 0 || report.Summary.Unknown > 0:
		report.OverallStatus = "completed_with_warnings"
	}
	return report, nil
}

func aggregateStatus(results []CheckResult) Status {
	status := StatusSkipped
	for _, result := range results {
		switch result.Status {
		case StatusFail:
			return StatusFail
		case StatusWarning:
			status = StatusWarning
		case StatusUnknown:
			if status != StatusWarning {
				status = StatusUnknown
			}
		case StatusPass:
			if status == StatusSkipped {
				status = StatusPass
			}
		}
	}
	return status
}
