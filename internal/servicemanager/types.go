package servicemanager

import "time"

type Scope string

const (
	ScopeSystem Scope = "system"
	ScopeUser   Scope = "user"
)

type Action string

const (
	ActionNone           Action = "none"
	ActionEnable         Action = "enable"
	ActionStart          Action = "start"
	ActionEnableAndStart Action = "enable_and_start"
	ActionDisable        Action = "disable"
	ActionStop           Action = "stop"
)

// ServiceRequest represents a component's requested service state.
type ServiceRequest struct {
	ComponentID string `json:"component_id"`
	UnitName    string `json:"unit_name"`
	Scope       Scope  `json:"scope"`
	Action      Action `json:"action"`
	Required    bool   `json:"required"`
}

// ServiceState describes current unit inspection results.
type ServiceState struct {
	UnitName      string `json:"unit_name"`
	Scope         Scope  `json:"scope"`
	Exists        bool   `json:"exists"`
	Loaded        bool   `json:"loaded"`
	ActiveState   string `json:"active_state"`    // "active", "inactive", "failed", "unavailable"
	UnitFileState string `json:"unit_file_state"` // "enabled", "disabled", "static", "masked", etc.
	Active        bool   `json:"active"`
	Enabled       bool   `json:"enabled"`
}

// ProposedChange details an anticipated systemctl mutation.
type ProposedChange struct {
	ComponentID string       `json:"component_id"`
	UnitName    string       `json:"unit_name"`
	Scope       Scope        `json:"scope"`
	Action      Action       `json:"action"`
	BeforeState ServiceState `json:"before_state"`
	WillMutate  bool         `json:"will_mutate"`
	Command     []string     `json:"command,omitempty"`
}

// ServicePlan represents the collection of planned service changes before execution.
type ServicePlan struct {
	SystemdAvailable bool             `json:"systemd_available"`
	ProposedChanges  []ProposedChange `json:"proposed_changes"`
	Warnings         []string         `json:"warnings,omitempty"`
	Errors           []string         `json:"errors,omitempty"`
}

// ServiceResult summarizes executed service modifications.
type ServiceResult struct {
	OperationID string         `json:"operation_id"`
	Timestamp   time.Time      `json:"timestamp"`
	Applied     []JournalEntry `json:"applied"`
	Simulated   bool           `json:"simulated"`
	Success     bool           `json:"success"`
	Errors      []string       `json:"errors,omitempty"`
	Warnings    []string       `json:"warnings,omitempty"`
}

// JournalEntry records a service modification for recovery and history.
type JournalEntry struct {
	OperationID string    `json:"operation_id"`
	Timestamp   time.Time `json:"timestamp"`
	ComponentID string    `json:"component_id"`
	UnitName    string    `json:"unit_name"`
	Scope       Scope     `json:"scope"`
	Action      Action    `json:"action"`
	Previous    struct {
		Active  bool   `json:"active"`
		Enabled bool   `json:"enabled"`
		State   string `json:"state"`
	} `json:"previous"`
	Status string `json:"status"` // "applied", "rolled_back", "failed"
}
