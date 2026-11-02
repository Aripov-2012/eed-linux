package snapshot

import "context"

// DefaultBackendName is the supported EED snapshot backend for the current milestone.
const DefaultBackendName = "btrfs_snapper"

// BackendHints expose basic backend capabilities used by the CLI if a specific implementation is not selected.
type BackendHints struct {
	Name               string
	SupportsRollback   bool
	RequiresPrivileges bool
}

// ProbeBackend performs a backend-agnostic capability probe.
func ProbeBackend(ctx context.Context, backend Backend) Capability {
	if backend == nil {
		return Capability{Available: false, Status: "unknown", Reason: "backend is nil"}
	}
	return backend.Probe(ctx)
}
