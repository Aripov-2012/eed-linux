package planner

import (
	"fmt"
	"strings"
)

func FormatPlan(plan Plan) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Plan status: %s\n", plan.Status)
	fmt.Fprintf(&b, "Profile: %s\n", plan.Profile)
	fmt.Fprintf(&b, "Target OS: %s\n", plan.TargetOS)
	fmt.Fprintf(&b, "Host OS: %s\n", plan.HostOS)
	fmt.Fprintf(&b, "Installable: %t\n", plan.Installable)
	if len(plan.Components) == 0 {
		fmt.Fprintln(&b, "Components: none")
	} else {
		fmt.Fprintln(&b, "Components:")
		for _, item := range plan.Components {
			fmt.Fprintf(&b, "  - %s [%s] (%s)\n", item.ID, item.Status, item.Reason)
		}
	}
	if len(plan.Packages) > 0 {
		fmt.Fprintf(&b, "Packages: %s\n", strings.Join(plan.Packages, ", "))
	}
	if len(plan.Warnings) > 0 {
		fmt.Fprintln(&b, "Warnings:")
		for _, warning := range plan.Warnings {
			fmt.Fprintf(&b, "  - %s\n", warning)
		}
	}
	if len(plan.Errors) > 0 {
		fmt.Fprintln(&b, "Errors:")
		for _, err := range plan.Errors {
			fmt.Fprintf(&b, "  - %s\n", err)
		}
	}
	return b.String()
}
