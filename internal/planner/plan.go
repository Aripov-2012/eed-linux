package planner

import (
	"encoding/json"

	"eed/internal/packageinfo"
)

// ComponentPlan describes a resolved component in the generated plan.
type ComponentPlan struct {
	ID           string   `json:"id"`
	Name         string   `json:"name"`
	Selected     bool     `json:"selected"`
	Reason       string   `json:"reason"`
	Status       string   `json:"status"`
	Dependencies []string `json:"dependencies,omitempty"`
}

// Plan is a machine-readable installation plan returned by the planner.
type Plan struct {
	SchemaVersion   int                     `json:"schema_version"`
	Profile         string                  `json:"profile"`
	TargetOS        string                  `json:"target_os"`
	HostOS          string                  `json:"host_os"`
	Installable     bool                    `json:"installable"`
	Components      []ComponentPlan         `json:"components"`
	Packages        []string                `json:"packages"`
	PackageStatuses []packageinfo.Candidate `json:"package_statuses,omitempty"`
	Warnings        []string                `json:"warnings,omitempty"`
	Errors          []string                `json:"errors,omitempty"`
	Status          string                  `json:"status"`
}

func (p Plan) MarshalJSON() ([]byte, error) {
	type alias Plan
	return json.Marshal(alias(p))
}
