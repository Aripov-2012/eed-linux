package snapshot

// Capability describes whether a backend can safely create snapshots and whether rollback is possible.
type Capability struct {
	Available           bool   `json:"available"`
	ToolInstalled       bool   `json:"tool_installed"`
	ConfigurationExists bool   `json:"configuration_exists"`
	CreationSupported   bool   `json:"creation_supported"`
	Backend             string `json:"backend,omitempty"`
	Status              string `json:"status,omitempty"`
	Reason              string `json:"reason,omitempty"`
	RequiresPrivileges  bool   `json:"requires_privileges"`
	SupportsRollback    bool   `json:"supports_rollback"`
}
