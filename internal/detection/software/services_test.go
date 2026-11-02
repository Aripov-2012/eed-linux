package software

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type serviceCommandRunner struct {
	calls        [][]string
	displayAlias string
}

func (r *serviceCommandRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	call := append([]string{name}, args...)
	r.calls = append(r.calls, call)
	if name != "systemctl" {
		return nil, errors.New("unexpected command")
	}
	joined := strings.Join(args, " ")
	user := strings.Contains(joined, "--user")
	if strings.Contains(joined, "list-unit-files") {
		if user {
			return []byte("pipewire.service disabled\nwireplumber.service static\n"), nil
		}
		return []byte("NetworkManager.service enabled\nbluetooth.service disabled\npipewire.service disabled\nsddm.service enabled\n"), nil
	}
	if strings.Contains(joined, "--value") && strings.HasSuffix(joined, "display-manager.service") {
		if r.displayAlias == "" {
			return nil, errors.New("no active display manager alias")
		}
		return []byte(r.displayAlias + "\n"), nil
	}
	unit := args[len(args)-1]
	loadState, activeState := "not-found", "inactive"
	switch unit {
	case "NetworkManager.service":
		loadState, activeState = "loaded", "active"
	case "bluetooth.service":
		loadState, activeState = "loaded", "failed"
	case "pipewire.service":
		loadState, activeState = "loaded", "inactive"
	case "sddm.service":
		loadState, activeState = "loaded", "active"
	case "wireplumber.service":
		if user {
			loadState, activeState = "loaded", "active"
		}
	case "gdm.service":
		loadState, activeState = "loaded", "active"
	}
	return []byte("Id=" + unit + "\nLoadState=" + loadState + "\nActiveState=" + activeState + "\n"), nil
}

func TestDetectServicesSkipsNonSystemdHosts(t *testing.T) {
	lookups := 0
	services, systemdAvailable, warnings := detectServices(DetectOptions{
		InitSystem: "openrc",
		LookPath: func(string) (string, error) {
			lookups++
			return "", errors.New("must not be called")
		},
	})
	if systemdAvailable || len(services) != 0 || len(warnings) != 0 || lookups != 0 {
		t.Fatalf("non-systemd hosts must not run systemctl probes: services=%+v available=%t warnings=%v lookups=%d",
			services, systemdAvailable, warnings, lookups)
	}
}

func TestDetectServicesReportsUnitStateAliasesAndScope(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "/run/user/1000")
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "unix:path=/run/user/1000/bus")
	runner := &serviceCommandRunner{displayAlias: "sddm.service"}
	services, systemdAvailable, warnings := detectServices(DetectOptions{
		InitSystem: "systemd", Runner: runner,
		LookPath: func(name string) (string, error) {
			if name != "systemctl" {
				t.Fatalf("unexpected executable lookup: %s", name)
			}
			return "/usr/bin/systemctl", nil
		},
	})
	if !systemdAvailable || len(services) != 8 || len(warnings) != 0 {
		t.Fatalf("unexpected systemd detection: services=%+v available=%t warnings=%v", services, systemdAvailable, warnings)
	}
	byKey := map[string]ServiceInfo{}
	for _, service := range services {
		byKey[service.Scope+":"+service.Name] = service
	}
	if got := byKey["system:sddm.service"]; got.Role != "display-manager" ||
		got.Alias != "display-manager.service" || !got.Installed || !got.Loaded || got.State != "active" {
		t.Errorf("display-manager alias should resolve to the installed, loaded active unit: %+v", got)
	}
	if got := byKey["system:bluetooth.service"]; !got.Installed || !got.Loaded || got.State != "failed" {
		t.Errorf("failed service state should remain distinct: %+v", got)
	}
	if got := byKey["system:pipewire.service"]; got.State != "inactive" || !got.Installed || !got.Loaded {
		t.Errorf("inactive service should not be reported active: %+v", got)
	}
	if got := byKey["system:docker.service"]; got.State != "unavailable" || got.Installed || got.Loaded || got.Available {
		t.Errorf("missing unit should be reported unavailable: %+v", got)
	}
	if got := byKey["user:wireplumber.service"]; got.State != "active" || got.Scope != "user" {
		t.Errorf("user service was not distinguished from system services: %+v", got)
	}
	for _, call := range runner.calls {
		if call[0] != "systemctl" {
			t.Errorf("unexpected command: %v", call)
		}
		if strings.Contains(strings.Join(call, " "), " start ") ||
			strings.Contains(strings.Join(call, " "), " stop ") ||
			strings.Contains(strings.Join(call, " "), " enable ") ||
			strings.Contains(strings.Join(call, " "), " disable ") {
			t.Errorf("service detection must remain read-only: %v", call)
		}
	}
}

func TestDetectServicesUsesGNOMEDisplayManagerConfiguration(t *testing.T) {
	configPath := filepath.Join(t.TempDir(), "default-display-manager")
	if err := os.WriteFile(configPath, []byte("/usr/sbin/gdm3\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := &serviceCommandRunner{}
	services, _, warnings := detectServices(DetectOptions{
		InitSystem: "systemd", Runner: runner, DefaultDisplayManagerPath: configPath,
		LookPath: func(string) (string, error) { return "/usr/bin/systemctl", nil },
	})
	if len(warnings) != 0 {
		t.Fatalf("unexpected detection warnings: %v", warnings)
	}
	for _, service := range services {
		if service.Name == "gdm.service" {
			if service.Role != "display-manager" || service.State != "active" {
				t.Fatalf("GNOME display-manager config was not used: %+v", service)
			}
			return
		}
	}
	t.Fatalf("expected gdm.service from default-display-manager config: %+v", services)
}

func TestSoftwareReportUsesActiveDisplayManagerAlias(t *testing.T) {
	t.Setenv("XDG_RUNTIME_DIR", "")
	t.Setenv("DBUS_SESSION_BUS_ADDRESS", "")
	report, _, err := Detect(DetectOptions{
		InitSystem: "systemd", Runner: &serviceCommandRunner{displayAlias: "sddm.service"},
		LookPath: func(name string) (string, error) {
			if name == "systemctl" {
				return "/usr/bin/systemctl", nil
			}
			return "", errors.New("not installed")
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.DisplayManager != "sddm" {
		t.Fatalf("active display manager should be reflected in the software report: %+v", report)
	}
}
