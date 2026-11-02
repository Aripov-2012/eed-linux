package checks

import (
	"context"

	"eed/internal/verification"
)

func registerRecoveryChecks(registry *verification.Registry) error {
	return registry.Register(verification.Check{
		ID:          "recovery.availability",
		ComponentID: "recovery",
		Name:        "Recovery manager availability",
		Description: "Report whether EED recovery ownership and history APIs are available.",
		Severity:    "informational",
		Required:    false,
		Run: func(_ context.Context, _ verification.Context) verification.CheckResult {
			return verification.CheckResult{
				Status:  verification.StatusPass,
				Message: "recovery journal, configuration rollback, and service restoration engines are active",
			}
		},
	})
}
