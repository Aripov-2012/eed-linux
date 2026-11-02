package verification

import "time"

type Status string

const (
	StatusPass    Status = "PASS"
	StatusWarning Status = "WARNING"
	StatusFail    Status = "FAIL"
	StatusUnknown Status = "UNKNOWN"
	StatusSkipped Status = "SKIPPED"
)

type CheckResult struct {
	ID          string `json:"id"`
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
	Severity    string `json:"severity,omitempty"`
	Required    bool   `json:"required"`
	Status      Status `json:"status"`
	Message     string `json:"message,omitempty"`
}

type ComponentResult struct {
	ComponentID string        `json:"component_id"`
	Status      Status        `json:"status"`
	Checks      []CheckResult `json:"checks"`
}

type Summary struct {
	Passed   int `json:"passed"`
	Warnings int `json:"warnings"`
	Failed   int `json:"failed"`
	Unknown  int `json:"unknown"`
	Skipped  int `json:"skipped"`
}

type Report struct {
	SchemaVersion        string            `json:"schema_version"`
	CollectedAt          time.Time         `json:"collected_at"`
	OverallStatus        string            `json:"overall_status"`
	Summary              Summary           `json:"summary"`
	Components           []ComponentResult `json:"components"`
	RecoveryAvailability string            `json:"recovery_availability,omitempty"`
	Warnings             []string          `json:"warnings,omitempty"`
	Errors               []string          `json:"errors,omitempty"`
}
