package checks

import (
	"context"
	"fmt"
	"os"
	"strings"

	"eed/internal/verification"
)

func registerServiceChecks(registry *verification.Registry) error {
	return registry.Register(verification.Check{
		ID:          "developer_docker.daemon",
		ComponentID: "developer.docker",
		Name:        "Docker daemon",
		Description: "Check whether the Docker daemon is reachable without changing service state.",
		Severity:    "optional",
		Required:    false,
		Run: func(ctx context.Context, input verification.Context) verification.CheckResult {
			if os.Geteuid() == 0 {
				return verification.CheckResult{Status: verification.StatusUnknown, Message: "daemon reachability from the invoking user's permissions cannot be assessed while running as root"}
			}
			output, err := input.Runner.Run(ctx, "docker", "info", "--format", "{{.ServerVersion}}")
			if err != nil {
				if ctx.Err() != nil {
					return verification.CheckResult{Status: verification.StatusUnknown, Message: fmt.Sprintf("Docker daemon check did not complete: %v", ctx.Err())}
				}
				return verification.CheckResult{Status: verification.StatusWarning, Message: fmt.Sprintf("Docker CLI is installed but daemon is not reachable: %s", strings.TrimSpace(string(output)))}
			}
			return verification.CheckResult{Status: verification.StatusPass, Message: "Docker daemon is reachable; server version " + strings.TrimSpace(string(output))}
		},
	})
}
