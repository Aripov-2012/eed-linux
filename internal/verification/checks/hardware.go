package checks

import (
	"context"

	"eed/internal/verification"
)

func registerHardwareChecks(registry *verification.Registry) error {
	return registry.Register(verification.Check{
		ID:          "desktop_hyprland.hardware",
		ComponentID: "desktop.hyprland",
		Name:        "GPU detection",
		Description: "Inspect whether the Detection Engine can identify a GPU.",
		Severity:    "optional",
		Required:    false,
		Run: func(_ context.Context, input verification.Context) verification.CheckResult {
			if len(input.Detection.Hardware.GPUs) == 0 {
				return verification.CheckResult{Status: verification.StatusUnknown, Message: "no GPU information was available from the Detection Engine"}
			}
			return verification.CheckResult{Status: verification.StatusPass, Message: "GPU information is available"}
		},
	})
}
