package servicemanager

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"
)

type fakeRunner struct {
	responses map[string][]byte
	errors    map[string]error
	calls     [][]string
}

func (r *fakeRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	call := append([]string{name}, args...)
	r.calls = append(r.calls, call)
	key := strings.Join(call, " ")
	return r.responses[key], r.errors[key]
}

func TestInspectAndPlanSystemServices(t *testing.T) {
	runner := &fakeRunner{
		responses: map[string][]byte{
			"systemctl --system show --property=Id,LoadState,ActiveState,UnitFileState -- docker.service": []byte("Id=docker.service\nLoadState=loaded\nActiveState=inactive\nUnitFileState=disabled\n"),
			"systemctl --user show --property=Id,LoadState,ActiveState,UnitFileState -- pipewire.service": []byte("Id=pipewire.service\nLoadState=loaded\nActiveState=active\nUnitFileState=enabled\n"),
		},
	}

	mgr, err := NewManager(Options{
		Runner:      runner,
		InitSystem:  "systemd",
		JournalPath: filepath.Join(t.TempDir(), "journal.json"),
		LookPath:    func(string) (string, error) { return "/usr/bin/systemctl", nil },
		UseSudo:     true,
	})
	if err != nil {
		t.Fatal(err)
	}

	requests := []ServiceRequest{
		{
			ComponentID: "developer.docker",
			UnitName:    "docker.service",
			Scope:       ScopeSystem,
			Action:      ActionEnableAndStart,
			Required:    true,
		},
		{
			ComponentID: "system.audio",
			UnitName:    "pipewire.service",
			Scope:       ScopeUser,
			Action:      ActionEnableAndStart,
			Required:    false,
		},
	}

	plan, err := mgr.Plan(context.Background(), requests)
	if err != nil {
		t.Fatal(err)
	}
	if !plan.SystemdAvailable || len(plan.ProposedChanges) != 2 {
		t.Fatalf("unexpected plan: %+v", plan)
	}

	// docker.service should mutate
	if !plan.ProposedChanges[0].WillMutate {
		t.Fatal("docker.service should require mutation")
	}
	wantDockerCmd := "sudo systemctl --system enable --now docker.service"
	if strings.Join(plan.ProposedChanges[0].Command, " ") != wantDockerCmd {
		t.Fatalf("expected command %q, got %q", wantDockerCmd, strings.Join(plan.ProposedChanges[0].Command, " "))
	}

	// pipewire.service is already active and enabled, should NOT mutate
	if plan.ProposedChanges[1].WillMutate {
		t.Fatal("pipewire.service should not require mutation")
	}
}

func TestMissingUnitHandling(t *testing.T) {
	runner := &fakeRunner{
		responses: map[string][]byte{
			"systemctl --system show --property=Id,LoadState,ActiveState,UnitFileState -- missing.service":  []byte("Id=missing.service\nLoadState=not-found\nActiveState=inactive\nUnitFileState=\n"),
			"systemctl --system show --property=Id,LoadState,ActiveState,UnitFileState -- optional.service": []byte("Id=optional.service\nLoadState=not-found\nActiveState=inactive\nUnitFileState=\n"),
		},
	}

	mgr, err := NewManager(Options{
		Runner:      runner,
		InitSystem:  "systemd",
		JournalPath: filepath.Join(t.TempDir(), "journal.json"),
		LookPath:    func(string) (string, error) { return "/usr/bin/systemctl", nil },
	})
	if err != nil {
		t.Fatal(err)
	}

	requests := []ServiceRequest{
		{ComponentID: "c1", UnitName: "missing.service", Scope: ScopeSystem, Action: ActionStart, Required: true},
		{ComponentID: "c2", UnitName: "optional.service", Scope: ScopeSystem, Action: ActionStart, Required: false},
	}

	plan, err := mgr.Plan(context.Background(), requests)
	if err != nil {
		t.Fatal(err)
	}

	if len(plan.Errors) != 1 || !strings.Contains(plan.Errors[0], "required service unit missing.service does not exist") {
		t.Fatalf("expected error for required missing unit: %v", plan.Errors)
	}
	if len(plan.Warnings) != 1 || !strings.Contains(plan.Warnings[0], "optional service unit optional.service does not exist") {
		t.Fatalf("expected warning for optional missing unit: %v", plan.Warnings)
	}
}

func TestDryRunDoesNotMutate(t *testing.T) {
	runner := &fakeRunner{
		responses: map[string][]byte{
			"systemctl --system show --property=Id,LoadState,ActiveState,UnitFileState -- test.service": []byte("Id=test.service\nLoadState=loaded\nActiveState=inactive\nUnitFileState=disabled\n"),
		},
	}

	mgr, err := NewManager(Options{
		Runner:      runner,
		InitSystem:  "systemd",
		JournalPath: filepath.Join(t.TempDir(), "journal.json"),
		LookPath:    func(string) (string, error) { return "/usr/bin/systemctl", nil },
	})
	if err != nil {
		t.Fatal(err)
	}

	plan, err := mgr.Plan(context.Background(), []ServiceRequest{
		{ComponentID: "c1", UnitName: "test.service", Scope: ScopeSystem, Action: ActionEnable, Required: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	initialCalls := len(runner.calls)
	result, err := mgr.Apply(context.Background(), "op-dry", plan, true, true)
	if err != nil {
		t.Fatal(err)
	}
	if !result.Simulated || !result.Success {
		t.Fatalf("expected simulated success: %+v", result)
	}
	if len(runner.calls) != initialCalls {
		t.Fatalf("dry-run must not execute mutation commands, calls were: %#v", runner.calls[initialCalls:])
	}
}

func TestApplyAndRollback(t *testing.T) {
	runner := &fakeRunner{
		responses: map[string][]byte{
			"systemctl --system show --property=Id,LoadState,ActiveState,UnitFileState -- s1.service": []byte("Id=s1.service\nLoadState=loaded\nActiveState=inactive\nUnitFileState=disabled\n"),
			"systemctl --system enable --now s1.service":                                              []byte("Created symlink ...\n"),
			"systemctl --system stop s1.service":                                                      []byte(""),
			"systemctl --system disable s1.service":                                                   []byte("Removed symlink ...\n"),
		},
	}

	mgr, err := NewManager(Options{
		Runner:      runner,
		InitSystem:  "systemd",
		JournalPath: filepath.Join(t.TempDir(), "journal.json"),
		LookPath:    func(string) (string, error) { return "/usr/bin/systemctl", nil },
		UseSudo:     false,
	})
	if err != nil {
		t.Fatal(err)
	}

	plan, err := mgr.Plan(context.Background(), []ServiceRequest{
		{ComponentID: "c1", UnitName: "s1.service", Scope: ScopeSystem, Action: ActionEnableAndStart, Required: true},
	})
	if err != nil {
		t.Fatal(err)
	}

	// Must fail without approval
	_, err = mgr.Apply(context.Background(), "op-app", plan, false, false)
	if err == nil {
		t.Fatal("expected error without approval")
	}

	// Apply with approval
	res, err := mgr.Apply(context.Background(), "op-app", plan, true, false)
	if err != nil {
		t.Fatalf("apply failed: %v", err)
	}
	if !res.Success || len(res.Applied) != 1 {
		t.Fatalf("expected 1 applied change: %+v", res)
	}

	// Rollback
	if err := mgr.Rollback(context.Background(), "op-app"); err != nil {
		t.Fatalf("rollback failed: %v", err)
	}

	// Verify rollback issued stop and disable
	var stopFound, disableFound bool
	for _, call := range runner.calls {
		cmdStr := strings.Join(call, " ")
		if strings.Contains(cmdStr, "stop s1.service") {
			stopFound = true
		}
		if strings.Contains(cmdStr, "disable s1.service") {
			disableFound = true
		}
	}
	if !stopFound || !disableFound {
		t.Fatalf("rollback commands missing: stop=%t disable=%t calls=%#v", stopFound, disableFound, runner.calls)
	}
}

func TestSystemdUnavailableFallback(t *testing.T) {
	mgr, err := NewManager(Options{
		InitSystem:  "sysvinit",
		LookPath:    func(string) (string, error) { return "", errors.New("not found") },
		JournalPath: filepath.Join(t.TempDir(), "journal.json"),
	})
	if err != nil {
		t.Fatal(err)
	}

	if mgr.IsSystemdAvailable(context.Background()) {
		t.Fatal("expected systemd to be reported unavailable")
	}

	plan, err := mgr.Plan(context.Background(), []ServiceRequest{
		{ComponentID: "c1", UnitName: "docker.service", Scope: ScopeSystem, Action: ActionStart, Required: true},
	})
	if err != nil {
		t.Fatal(err)
	}
	if plan.SystemdAvailable || len(plan.Warnings) == 0 {
		t.Fatalf("expected plan warning for unavailable systemd: %+v", plan)
	}
}
