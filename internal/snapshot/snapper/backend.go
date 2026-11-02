package snapper

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"time"

	"eed/internal/snapshot"
)

// Runner is the command runner used by the backend.
type Runner interface {
	Run(context.Context, string, ...string) ([]byte, error)
}

// ExecRunner executes external commands.
type ExecRunner struct{}

// Run runs a command and captures stdout/stderr.
func (ExecRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	return cmd.Output()
}

// Backend implements the snapshot backend for Snapper.
type Backend struct {
	Runner     Runner
	LookPath   func(string) (string, error)
	IsElevated func() bool
	ConfigName string
	Timeout    time.Duration
	WorkingDir string
}

// NewBackend creates a new Snapper backend.
func NewBackend() *Backend {
	return &Backend{
		Runner:     ExecRunner{},
		LookPath:   exec.LookPath,
		IsElevated: func() bool { return os.Geteuid() == 0 },
		ConfigName: "root",
		Timeout:    15 * time.Second,
	}
}

// Name returns the backend name.
func (b *Backend) Name() string { return "btrfs_snapper" }

// Probe checks whether Snapper is present and whether a valid configuration exists.
func (b *Backend) Probe(ctx context.Context) snapshot.Capability {
	cap := snapshot.Capability{Backend: b.Name(), Status: "unknown", RequiresPrivileges: true, SupportsRollback: false}
	runner := b.runner()
	lookPath := b.lookPath()
	if _, err := lookPath("snapper"); err != nil {
		cap.Available = false
		cap.Status = "snapper_missing"
		cap.Reason = "snapper executable is not installed"
		return cap
	}
	cap.ToolInstalled = true
	probeCtx, cancel := b.withTimeout(ctx)
	defer cancel()
	data, err := runner.Run(probeCtx, "snapper", "list-configs")
	if err != nil {
		cap.Available = false
		cap.Status = "configuration_missing"
		cap.Reason = fmt.Sprintf("unable to list Snapper configurations: %v", err)
		return cap
	}
	configs := ParseConfigs(string(data))
	if len(configs) == 0 {
		cap.Available = false
		cap.Status = "configuration_missing"
		cap.Reason = "no Snapper configurations were found"
		return cap
	}
	configName := b.ConfigName
	if configName == "" {
		configName = "root"
	}
	found := false
	for _, config := range configs {
		if config.Name == configName {
			found = true
			break
		}
	}
	if !found {
		cap.Available = false
		cap.Status = "configuration_missing"
		cap.Reason = fmt.Sprintf("Snapper configuration %q was not found", configName)
		return cap
	}
	cap.ConfigurationExists = true
	cap.Available = true
	cap.CreationSupported = true
	cap.Status = "configured_unverified"
	cap.Reason = fmt.Sprintf("Snapper configuration %q detected; snapshot creation and full rollback are not verified", configName)
	return cap
}

// List lists snapshots for the selected configuration.
func (b *Backend) List(ctx context.Context, configName string) ([]snapshot.Snapshot, error) {
	if configName == "" {
		configName = b.ConfigName
	}
	if configName == "" {
		return nil, fmt.Errorf("snapshot configuration is required")
	}
	if _, err := b.lookPath()("snapper"); err != nil {
		return nil, fmt.Errorf("snapper executable is unavailable: %w", err)
	}
	runCtx, cancel := b.withTimeout(ctx)
	defer cancel()
	data, err := b.runner().Run(runCtx, "snapper", "-c", configName, "list")
	if err != nil {
		return nil, err
	}
	return ParseSnapshotList(string(data))
}

// Create creates a read-only EED-owned snapshot using the selected Snapper configuration.
func (b *Backend) Create(ctx context.Context, req snapshot.CreateRequest) (snapshot.Snapshot, error) {
	if req.ConfigName == "" {
		req.ConfigName = b.ConfigName
	}
	if req.ConfigName == "" {
		return snapshot.Snapshot{}, fmt.Errorf("snapshot configuration is required")
	}
	if !req.ReadOnly {
		return snapshot.Snapshot{}, fmt.Errorf("EED only supports creating read-only snapshots")
	}
	isElevated := b.IsElevated
	if isElevated == nil {
		isElevated = func() bool { return os.Geteuid() == 0 }
	}
	if !isElevated() {
		return snapshot.Snapshot{}, fmt.Errorf("snapshot creation requires explicit elevated invocation; run only this command with sudo")
	}
	if req.Type == "" {
		req.Type = "single"
	}
	if req.UserData == nil {
		req.UserData = map[string]string{"eed": "true"}
	} else {
		req.UserData["eed"] = "true"
	}
	if _, err := b.lookPath()("snapper"); err != nil {
		return snapshot.Snapshot{}, fmt.Errorf("snapper executable is unavailable: %w", err)
	}
	listConfigsCtx, cancelListConfigs := b.withTimeout(ctx)
	configData, err := b.runner().Run(listConfigsCtx, "snapper", "list-configs")
	cancelListConfigs()
	if err != nil {
		return snapshot.Snapshot{}, fmt.Errorf("inspect Snapper configurations: %w", err)
	}
	if !hasConfig(ParseConfigs(string(configData)), req.ConfigName) {
		return snapshot.Snapshot{}, fmt.Errorf("Snapper configuration %q does not exist", req.ConfigName)
	}
	args := BuildCreateArgs(req)
	createCtx, cancelCreate := b.withTimeout(ctx)
	data, err := b.runner().Run(createCtx, "snapper", args...)
	cancelCreate()
	if err != nil {
		return snapshot.Snapshot{}, err
	}
	created, err := ParseCreateOutput(strings.TrimSpace(string(data)))
	if err != nil {
		return snapshot.Snapshot{}, err
	}
	listed, err := b.List(ctx, req.ConfigName)
	if err != nil {
		return snapshot.Snapshot{}, fmt.Errorf("snapshot #%d may have been created, but verification failed: %w", created.Number, err)
	}
	for _, actual := range listed {
		if actual.Number == created.Number {
			actual.ReadOnly = true
			actual.EEDOwned = strings.EqualFold(actual.UserData["eed"], "true")
			if !actual.EEDOwned {
				return snapshot.Snapshot{}, fmt.Errorf("snapshot #%d exists but EED ownership metadata could not be verified", created.Number)
			}
			return actual, nil
		}
	}
	return snapshot.Snapshot{}, fmt.Errorf("snapper reported snapshot #%d created, but it was not present in the subsequent listing", created.Number)
}

func (b *Backend) runner() Runner {
	if b.Runner != nil {
		return b.Runner
	}
	return ExecRunner{}
}

func (b *Backend) lookPath() func(string) (string, error) {
	if b.LookPath != nil {
		return b.LookPath
	}
	return exec.LookPath
}

func (b *Backend) withTimeout(ctx context.Context) (context.Context, context.CancelFunc) {
	if ctx == nil {
		ctx = context.Background()
	}
	timeout := b.Timeout
	if timeout <= 0 {
		timeout = 15 * time.Second
	}
	return context.WithTimeout(ctx, timeout)
}

func hasConfig(configs []Config, expected string) bool {
	for _, config := range configs {
		if config.Name == expected {
			return true
		}
	}
	return false
}
