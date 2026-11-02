package orchestrator

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"eed/internal/components"
	"eed/internal/config"
	"eed/internal/configmanager"
	"eed/internal/detection"
	"eed/internal/packageinfo"
	"eed/internal/portalconfig"
	"eed/internal/servicemanager"
)

type mockExecRunner struct {
	responses map[string][]byte
	errors    map[string]error
	calls     [][]string
}

func (m *mockExecRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	call := append([]string{name}, args...)
	m.calls = append(m.calls, call)
	key := strings.Join(call, " ")
	return m.responses[key], m.errors[key]
}

func (m *mockExecRunner) LookPath(name string) (string, error) {
	return "/usr/bin/" + name, nil
}

func testArchDependencies(t *testing.T, runner *mockExecRunner) Dependencies {
	t.Helper()
	deps := testDependencies()
	deps.LoadRegistry = func(string) (*components.Registry, error) {
		return components.LoadFromDir("../../components/manifests")
	}
	deps.LoadConfig = func(string) (config.Config, error) {
		cfg := config.DefaultConfig()
		cfg.Developer.Git = true
		return cfg, nil
	}
	deps.Detect = func(context.Context) (detection.Report, error) {
		return detection.Report{
			System: detection.SystemReport{OS: detection.OSDetails{ID: "arch", Name: "Arch Linux", IsSupportedTarget: true}},
		}, nil
	}
	deps.InspectPackages = func(ctx context.Context, pkgs []string) (packageinfo.Report, error) {
		return packageinfo.Report{
			Status:         "available",
			InventoryKnown: true,
			Candidates: []packageinfo.Candidate{
				{Name: "git", State: packageinfo.StateNotInstalled, RepositoryStatus: "available"},
			},
		}, nil
	}
	deps.PlanPortals = func(desktop string, _ map[string]bool) (portalconfig.Plan, error) {
		return portalconfig.Plan{
			Desktop:          desktop,
			ConfigPath:       filepath.Join(t.TempDir(), "portals.conf"),
			Content:          "[preferred]\ndefault=gtk\n",
			PreserveExisting: false,
		}, nil
	}
	return deps
}

func TestExecuteInstallationRejectsNonArchHost(t *testing.T) {
	runner := &mockExecRunner{responses: map[string][]byte{}}
	deps := testDependencies()
	deps.LoadConfig = func(string) (config.Config, error) {
		cfg := config.DefaultConfig()
		cfg.Developer.Git = true
		return cfg, nil
	}
	deps.Detect = func(context.Context) (detection.Report, error) {
		return detection.Report{
			System: detection.SystemReport{OS: detection.OSDetails{ID: "ubuntu", Name: "Ubuntu", IsSupportedTarget: false}},
		}, nil
	}

	storageDir := t.TempDir()
	opts := InstallOptions{
		DryRun:         false,
		Confirmed:      true,
		JournalStorage: &JournalStorage{Directory: storageDir},
		Runner:         runner,
	}

	record, err := ExecuteInstallation(context.Background(), deps, opts)
	if err == nil {
		t.Fatal("expected error rejecting Ubuntu host")
	}
	if !strings.Contains(err.Error(), "current host operating system \"ubuntu\" is not Arch Linux") {
		t.Fatalf("unexpected error message: %v", err)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("no commands should be executed on non-Arch host: %#v", runner.calls)
	}
	if record.OverallStatus != "failed" {
		t.Fatalf("expected failed record status, got %q", record.OverallStatus)
	}
}

func TestExecuteInstallationDryRunIsCompletelyReadOnly(t *testing.T) {
	runner := &mockExecRunner{responses: map[string][]byte{}}
	deps := testArchDependencies(t, runner)

	storageDir := t.TempDir()
	opts := InstallOptions{
		DryRun:         true,
		Confirmed:      false,
		JournalStorage: &JournalStorage{Directory: storageDir},
		Runner:         runner,
	}

	record, err := ExecuteInstallation(context.Background(), deps, opts)
	if err != nil {
		t.Fatalf("unexpected dry-run error: %v", err)
	}
	if record.OverallStatus != "dry_run" {
		t.Fatalf("expected dry_run record status, got %q", record.OverallStatus)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("dry run must execute NO mutation commands: %#v", runner.calls)
	}
}

func TestExecuteInstallationCancellation(t *testing.T) {
	runner := &mockExecRunner{responses: map[string][]byte{}}
	deps := testArchDependencies(t, runner)

	storageDir := t.TempDir()
	opts := InstallOptions{
		DryRun:           false,
		Confirmed:        false,
		AllowUnprotected: true,
		Input:            strings.NewReader("NO\n"),
		JournalStorage:   &JournalStorage{Directory: storageDir},
		Runner:           runner,
	}

	record, err := ExecuteInstallation(context.Background(), deps, opts)
	if err == nil {
		t.Fatal("expected cancellation error")
	}
	if !strings.Contains(err.Error(), "transaction was not confirmed") {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("cancelled installation must execute no commands: %#v", runner.calls)
	}
	if record.OverallStatus != "failed" {
		t.Fatalf("expected failed record status: %+v", record)
	}
}

func TestExecuteInstallationRejectsUnprotectedSnapshot(t *testing.T) {
	runner := &mockExecRunner{responses: map[string][]byte{}}
	deps := testArchDependencies(t, runner)

	storageDir := t.TempDir()
	opts := InstallOptions{
		DryRun:           false,
		Confirmed:        false,
		AllowUnprotected: false,
		Input:            strings.NewReader("NO\n"),
		JournalStorage:   &JournalStorage{Directory: storageDir},
		Runner:           runner,
	}

	record, err := ExecuteInstallation(context.Background(), deps, opts)
	if err == nil {
		t.Fatal("expected rejection error")
	}
	if !strings.Contains(err.Error(), "user rejected unprotected installation") {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("rejected installation must execute no commands: %#v", runner.calls)
	}
	if record.OverallStatus != "failed" {
		t.Fatalf("expected failed record status: %+v", record)
	}
}

func TestExecuteInstallationEndToEndArchMocked(t *testing.T) {
	tempRoot := t.TempDir()
	configDir := filepath.Join(tempRoot, ".config")
	portalPath := filepath.Join(configDir, "xdg-desktop-portal", "portals.conf")

	runner := &mockExecRunner{
		responses: map[string][]byte{
			"pacman -Q":                   []byte("curl 8.0-1\n"),
			"pacman -Qe":                  []byte(""),
			"pacman -Si -- git":           []byte("Repository : extra\nName : git\nVersion : 2.0-1\nArchitecture : x86_64\nDownload Size : 1 MiB\nInstalled Size : 2 MiB\n"),
			"pacman -Syu --needed -- git": []byte("Targets (1) git-2.0-1\nTotal Installed Size: 2.00 MiB\n"),
			"systemctl --system show --property=Id,LoadState,ActiveState,UnitFileState -- docker.service": []byte("Id=docker.service\nLoadState=loaded\nActiveState=inactive\nUnitFileState=disabled\n"),
			"systemctl --system enable --now docker.service":                                              []byte("Created symlink ...\n"),
		},
	}

	deps := testArchDependencies(t, runner)
	deps.PlanPortals = func(desktop string, _ map[string]bool) (portalconfig.Plan, error) {
		return portalconfig.Plan{
			Desktop:          desktop,
			ConfigPath:       portalPath,
			Content:          "[preferred]\ndefault=gtk\n",
			PreserveExisting: false,
		}, nil
	}

	cfgMgr, err := configmanager.NewManager(configmanager.Options{
		ApprovedRoots: []string{configDir, tempRoot},
		JournalPath:   filepath.Join(tempRoot, "config_journal.json"),
		UserConfigDir: configDir,
		UserHome:      tempRoot,
	})
	if err != nil {
		t.Fatal(err)
	}

	svcMgr, err := servicemanager.NewManager(servicemanager.Options{
		Runner:      runner,
		InitSystem:  "systemd",
		JournalPath: filepath.Join(tempRoot, "service_journal.json"),
		LookPath:    func(string) (string, error) { return "/usr/bin/systemctl", nil },
		UseSudo:     false,
	})
	if err != nil {
		t.Fatal(err)
	}

	storageDir := filepath.Join(tempRoot, "installations")
	opts := InstallOptions{
		DryRun:           false,
		Confirmed:        true,
		AllowUnprotected: true,
		UseSudo:          false,
		JournalStorage:   &JournalStorage{Directory: storageDir},
		ConfigManager:    cfgMgr,
		ServiceManager:   svcMgr,
		Runner:           runner,
	}

	record, err := ExecuteInstallation(context.Background(), deps, opts)
	if err != nil {
		t.Fatalf("unexpected installation failure: %v", err)
	}
	if record.OverallStatus != "success" && record.OverallStatus != "completed_with_warnings" {
		t.Fatalf("expected success, got %q", record.OverallStatus)
	}

	// Verify portal config file was deployed
	portalData, err := os.ReadFile(portalPath)
	if err != nil {
		t.Fatalf("portal config file was not deployed: %v", err)
	}
	if !strings.Contains(string(portalData), "default=gtk") {
		t.Fatalf("unexpected portal content: %q", string(portalData))
	}

	// Verify journal was persisted
	savedRecord, err := opts.JournalStorage.LoadLatest()
	if err != nil {
		t.Fatalf("load latest journal failed: %v", err)
	}
	if savedRecord.OperationID != record.OperationID {
		t.Fatalf("saved operation ID mismatch: got %q, want %q", savedRecord.OperationID, record.OperationID)
	}
}

func TestExecuteInstallationPacmanFailureAbortsBeforeConfig(t *testing.T) {
	tempRoot := t.TempDir()
	configDir := filepath.Join(tempRoot, ".config")
	portalPath := filepath.Join(configDir, "xdg-desktop-portal", "portals.conf")

	runner := &mockExecRunner{
		responses: map[string][]byte{
			"pacman -Q":         []byte(""),
			"pacman -Qe":        []byte(""),
			"pacman -Si -- git": []byte("Repository : extra\nName : git\nVersion : 2.0-1\nArchitecture : x86_64\nDownload Size : 1 MiB\nInstalled Size : 2 MiB\n"),
		},
		errors: map[string]error{
			"pacman -Syu --needed -- git": errors.New("exit status 1"),
		},
	}

	deps := testArchDependencies(t, runner)
	deps.PlanPortals = func(desktop string, _ map[string]bool) (portalconfig.Plan, error) {
		return portalconfig.Plan{
			Desktop:          desktop,
			ConfigPath:       portalPath,
			Content:          "[preferred]\ndefault=gtk\n",
			PreserveExisting: false,
		}, nil
	}

	cfgMgr, err := configmanager.NewManager(configmanager.Options{
		ApprovedRoots: []string{configDir, tempRoot},
		JournalPath:   filepath.Join(tempRoot, "config_journal.json"),
		UserConfigDir: configDir,
		UserHome:      tempRoot,
	})
	if err != nil {
		t.Fatal(err)
	}

	storageDir := filepath.Join(tempRoot, "installations")
	opts := InstallOptions{
		DryRun:           false,
		Confirmed:        true,
		AllowUnprotected: true,
		UseSudo:          false,
		JournalStorage:   &JournalStorage{Directory: storageDir},
		ConfigManager:    cfgMgr,
		Runner:           runner,
	}

	record, err := ExecuteInstallation(context.Background(), deps, opts)
	if err == nil {
		t.Fatal("expected pacman failure error")
	}
	if record.OverallStatus != "failed" {
		t.Fatalf("expected failed status, got %q", record.OverallStatus)
	}

	// Verify that configuration was NOT deployed because pacman failed!
	if _, err := os.Stat(portalPath); !os.IsNotExist(err) {
		t.Fatal("configuration must NOT be deployed when package installation fails!")
	}
}
