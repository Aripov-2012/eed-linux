package configmanager

import (
	"time"
)

type ItemKind string

const (
	KindFile      ItemKind = "file"
	KindDirectory ItemKind = "directory"
	KindSymlink   ItemKind = "symlink"
	KindTemplate  ItemKind = "template"
)

// Item describes a configuration asset to be deployed.
type Item struct {
	Kind           ItemKind `json:"kind"`
	ComponentID    string   `json:"component_id"`
	Source         string   `json:"source,omitempty"`
	Destination    string   `json:"destination"`
	Content        []byte   `json:"-"`
	TemplateString string   `json:"-"`
	SymlinkTarget  string   `json:"symlink_target,omitempty"`
	FileMode       uint32   `json:"file_mode,omitempty"`
}

// PlanItem describes the anticipated action and safety assessment for an item.
type PlanItem struct {
	Item             Item     `json:"item"`
	Action           string   `json:"action"`           // "create", "backup_and_replace", "skip_identical", "error"
	DestinationType  string   `json:"destination_type"` // "none", "regular", "directory", "symlink", "other"
	Exists           bool     `json:"exists"`
	EEDOwned         bool     `json:"eed_owned"`
	ModifiedByUser   bool     `json:"modified_by_user"`
	BackupPath       string   `json:"backup_path,omitempty"`
	RequiresApproval bool     `json:"requires_approval"`
	Warnings         []string `json:"warnings,omitempty"`
}

// JournalEntry records ownership and deployment metadata for one deployed configuration path.
type JournalEntry struct {
	OperationID   string    `json:"operation_id"`
	Timestamp     time.Time `json:"timestamp"`
	ComponentID   string    `json:"component_id"`
	Kind          ItemKind  `json:"kind"`
	Destination   string    `json:"destination"`
	Source        string    `json:"source,omitempty"`
	BackupPath    string    `json:"backup_path,omitempty"`
	DeployedHash  string    `json:"deployed_hash,omitempty"`
	SymlinkTarget string    `json:"symlink_target,omitempty"`
	Status        string    `json:"status"` // "deployed", "rolled_back", "failed"
}

// DeploymentReport summarizes the results of a configuration deployment run.
type DeploymentReport struct {
	OperationID string         `json:"operation_id"`
	Timestamp   time.Time      `json:"timestamp"`
	Deployed    []JournalEntry `json:"deployed"`
	Backups     []string       `json:"backups"`
	Success     bool           `json:"success"`
	Errors      []string       `json:"errors,omitempty"`
	Warnings    []string       `json:"warnings,omitempty"`
}
