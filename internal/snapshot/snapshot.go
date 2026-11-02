package snapshot

import (
	"context"
	"time"
)

// Snapshot describes a single filesystem snapshot.
type Snapshot struct {
	Number      int               `json:"number"`
	Type        string            `json:"type,omitempty"`
	Description string            `json:"description,omitempty"`
	ReadOnly    bool              `json:"read_only"`
	CreatedAt   time.Time         `json:"created_at,omitempty"`
	UUID        string            `json:"uuid,omitempty"`
	UserData    map[string]string `json:"userdata,omitempty"`
	EEDOwned    bool              `json:"eed_owned"`
}

// CreateRequest describes a request to create a snapshot.
type CreateRequest struct {
	ConfigName  string            `json:"config_name,omitempty"`
	Description string            `json:"description,omitempty"`
	Type        string            `json:"type,omitempty"`
	ReadOnly    bool              `json:"read_only"`
	UserData    map[string]string `json:"userdata,omitempty"`
}

// Backend is the interface for backend implementations.
type Backend interface {
	Name() string
	Probe(context.Context) Capability
	List(context.Context, string) ([]Snapshot, error)
	Create(context.Context, CreateRequest) (Snapshot, error)
}
