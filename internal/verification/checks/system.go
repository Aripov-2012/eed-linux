package checks

import (
	"context"
	"strings"

	"eed/internal/verification"
)

func registerSystemChecks(registry *verification.Registry) error {
	return registry.Register(verification.Check{
		ID:          "system.host",
		ComponentID: "system",
		Name:        "Host detection",
		Description: "Summarize operating system and kernel information detected by EED.",
		Severity:    "informational",
		Required:    false,
		Run: func(_ context.Context, input verification.Context) verification.CheckResult {
			osName := strings.TrimSpace(input.Detection.System.OS.Name)
			kernel := strings.TrimSpace(input.Detection.System.Kernel.Release)
			if osName == "" || kernel == "" {
				return verification.CheckResult{Status: verification.StatusUnknown, Message: "operating system or kernel information is incomplete"}
			}
			if !input.Detection.System.OS.IsSupportedTarget {
				return verification.CheckResult{Status: verification.StatusWarning, Message: osName + " " + kernel + " detected; this host is outside EED's supported Arch Linux target"}
			}
			return verification.CheckResult{Status: verification.StatusPass, Message: osName + " " + kernel}
		},
	})
}
