package software

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

type packageManagerRunner struct {
	calls []string
}

func (r *packageManagerRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	r.calls = append(r.calls, name)
	switch name {
	case "apt":
		return []byte("apt 2.8.3 (amd64)\n"), nil
	case "pacman":
		return []byte("Pacman v6.1.0\n"), nil
	default:
		return nil, errors.New("unexpected command")
	}
}

func TestUbuntuReportsAPTAsNativeAndPacmanAsArchTarget(t *testing.T) {
	runner := &packageManagerRunner{}
	lookups := make([]string, 0)
	report, warnings := detectPackageManagers(DetectOptions{
		OSID:   "ubuntu",
		Runner: runner,
		LookPath: func(name string) (string, error) {
			lookups = append(lookups, name)
			if name == "apt" {
				return "/usr/bin/apt", nil
			}
			return "", errors.New("not installed")
		},
	})
	if len(warnings) != 0 {
		t.Fatalf("missing unrelated target manager should not create warnings: %v", warnings)
	}
	if len(report) != 2 || report[0].Name != "apt" || !report[0].Native || !report[0].Available ||
		report[0].Version != "apt 2.8.3 (amd64)" || report[0].MatchesTarget {
		t.Fatalf("expected apt as available native manager: %+v", report)
	}
	if report[1].Name != "pacman" || report[1].Available || report[1].Native || !report[1].MatchesTarget {
		t.Fatalf("expected pacman to be reported unavailable for the Arch target: %+v", report[1])
	}
	if report[0].HostDistribution != "ubuntu" || !report[0].HostNative ||
		report[0].ConfiguredTargetDistribution != "arch" ||
		report[0].TargetPackageManager != "pacman" || report[0].AdapterStatus != "implemented" {
		t.Fatalf("Ubuntu host/native and Arch target metadata should be independent: %+v", report[0])
	}
	if !reflect.DeepEqual(lookups, []string{"apt", "pacman", "dnf", "zypper", "apk"}) {
		t.Fatalf("should probe known managers without inferring the host distro from them: %v", lookups)
	}
	if !reflect.DeepEqual(runner.calls, []string{"apt"}) {
		t.Fatalf("should only query version for an available manager: %v", runner.calls)
	}
}

func TestArchReportsPacmanAsNativeAndTarget(t *testing.T) {
	report, warnings := detectPackageManagers(DetectOptions{
		OSID: "arch",
		LookPath: func(name string) (string, error) {
			if name == "pacman" {
				return "/usr/bin/pacman", nil
			}
			return "", errors.New("not installed")
		},
		Runner: &packageManagerRunner{},
	})
	if len(warnings) != 0 || len(report) != 1 {
		t.Fatalf("expected a single pacman record without warnings: report=%+v warnings=%v", report, warnings)
	}
	if !report[0].Native || !report[0].MatchesTarget || !report[0].Available {
		t.Fatalf("pacman should be both native and the EED installation target: %+v", report[0])
	}
	if report[0].HostDistribution != "arch" || report[0].ConfiguredTargetDistribution != "arch" ||
		report[0].TargetPackageManager != "pacman" || report[0].AdapterStatus != "implemented" {
		t.Fatalf("host, target, and adapter metadata should remain explicit: %+v", report[0])
	}
}

func TestDebianReportsAPTAsNative(t *testing.T) {
	report, warnings := detectPackageManagers(DetectOptions{
		OSID: "debian",
		LookPath: func(name string) (string, error) {
			if name == "apt" {
				return "/usr/bin/apt", nil
			}
			return "", errors.New("not installed")
		},
	})
	if len(warnings) != 0 || len(report) != 2 {
		t.Fatalf("unexpected Debian managers: report=%+v warnings=%v", report, warnings)
	}
	if !report[0].HostNative || !report[0].Native || report[0].Name != "apt" ||
		report[1].Name != "pacman" || report[1].HostNative || report[1].Native {
		t.Fatalf("Debian native manager must come from distribution identity: %+v", report)
	}
}

func TestMultipleInstalledManagersDoNotChangeHostIdentity(t *testing.T) {
	report, warnings := detectPackageManagers(DetectOptions{
		OSID: "ubuntu",
		LookPath: func(name string) (string, error) {
			switch name {
			case "apt", "pacman", "dnf":
				return "/usr/bin/" + name, nil
			default:
				return "", errors.New("not installed")
			}
		},
	})
	if len(warnings) != 0 || len(report) != 3 {
		t.Fatalf("expected all installed package managers, got report=%+v warnings=%v", report, warnings)
	}
	if report[0].Name != "apt" || !report[0].HostNative ||
		report[1].Name != "pacman" || !report[1].MatchesTarget || report[1].HostNative ||
		report[2].Name != "dnf" || report[2].HostNative || report[2].AdapterStatus != "not_implemented" {
		t.Fatalf("manager executable presence must not alter Ubuntu identity or native status: %+v", report)
	}
}
