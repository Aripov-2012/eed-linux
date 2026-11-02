package checks

import (
	"context"
	"fmt"
	"strings"

	"eed/internal/verification"
)

type executableCheck struct {
	component string
	binary    string
	args      []string
}

var executables = []executableCheck{
	{component: "developer.git", binary: "git", args: []string{"--version"}},
	{component: "developer.docker", binary: "docker", args: []string{"--version"}},
	{component: "developer.python", binary: "python3", args: []string{"--version"}},
	{component: "developer.nodejs", binary: "node", args: []string{"--version"}},
	{component: "developer.go", binary: "go", args: []string{"version"}},
	{component: "developer.rust", binary: "rustc", args: []string{"--version"}},
	{component: "editor.neovim", binary: "nvim", args: []string{"--version"}},
	{component: "editor.vscode", binary: "code", args: []string{"--version"}},
}

func registerPackageChecks(registry *verification.Registry) error {
	for _, executable := range executables {
		entry := executable
		id := strings.ReplaceAll(entry.component, ".", "_") + ".binary"
		err := registry.Register(verification.Check{
			ID:          id,
			ComponentID: entry.component,
			Name:        fmt.Sprintf("%s executable", entry.binary),
			Description: "Verify the selected component executable and version.",
			Severity:    "required",
			Required:    true,
			Run: func(ctx context.Context, input verification.Context) verification.CheckResult {
				path, err := input.Runner.LookPath(entry.binary)
				if err != nil {
					return verification.CheckResult{Status: verification.StatusFail, Message: fmt.Sprintf("required executable %q is not available", entry.binary)}
				}
				output, err := input.Runner.Run(ctx, entry.binary, entry.args...)
				if err != nil {
					return verification.CheckResult{Status: verification.StatusWarning, Message: fmt.Sprintf("%s is present at %s, but version could not be determined: %v", entry.binary, path, err)}
				}
				version := strings.TrimSpace(string(output))
				if version == "" {
					return verification.CheckResult{Status: verification.StatusUnknown, Message: fmt.Sprintf("%s is present, but returned no version information", entry.binary)}
				}
				return verification.CheckResult{Status: verification.StatusPass, Message: fmt.Sprintf("%s (%s)", path, firstLine(version))}
			},
		})
		if err != nil {
			return err
		}
	}
	return nil
}

func firstLine(value string) string {
	if i := strings.IndexByte(value, '\n'); i >= 0 {
		return strings.TrimSpace(value[:i])
	}
	return value
}
