package detection

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	hw "eed/internal/detection/hardware"
	sw "eed/internal/detection/software"
	sys "eed/internal/detection/system"
)

type fakeRunner struct{}

func (fakeRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	switch name {
	case "systemd-detect-virt":
		return []byte("none\n"), nil
	default:
		return nil, os.ErrNotExist
	}
}

func TestSystemOSDetection(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "os-release")
	content := "NAME=\"Arch Linux\"\nID=arch\nVERSION_ID=\"2024.01\"\nPRETTY_NAME=\"Arch Linux\"\nID_LIKE=\"arch\"\n"
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write os-release: %v", err)
	}
	info, _, err := sys.Detect(sys.DetectOptions{OSReleasePath: path, ProcRoot: "/proc", SysfsRoot: "/sys", HomeDir: "/tmp", Runner: fakeRunner{}})
	if err != nil {
		t.Fatalf("system detect failed: %v", err)
	}
	if info.OS.ID != "arch" {
		t.Fatalf("unexpected os id: %s", info.OS.ID)
	}
	if !info.OS.IsSupportedTarget {
		t.Fatal("arch should be detected as supported target")
	}
}

func TestHardwareCPUAndMemoryParsing(t *testing.T) {
	dir := t.TempDir()
	procDir := filepath.Join(dir, "proc")
	sysDir := filepath.Join(dir, "sys")
	if err := os.MkdirAll(procDir, 0o755); err != nil {
		t.Fatalf("mkdir proc: %v", err)
	}
	if err := os.MkdirAll(filepath.Join(sysDir, "class", "dmi", "id"), 0o755); err != nil {
		t.Fatalf("mkdir sys: %v", err)
	}
	cpuInfo := `processor	: 0
vendor_id	: GenuineIntel
cpu family	: 6
model		: 142
model name	: Intel(R) Core(TM) i7-1165G7 CPU @ 2.80GHz
stepping	: 1
microcode	: 0x1
cpu MHz		: 2800.000
cpu cores	: 4
siblings	: 8
processor	: 1
vendor_id	: GenuineIntel
cpu family	: 6
model		: 142
model name	: Intel(R) Core(TM) i7-1165G7 CPU @ 2.80GHz
stepping	: 1
microcode	: 0x1
cpu MHz		: 2800.000
cpu cores	: 4
siblings	: 8
`
	if err := os.WriteFile(filepath.Join(procDir, "cpuinfo"), []byte(cpuInfo), 0o600); err != nil {
		t.Fatalf("write cpuinfo: %v", err)
	}
	memInfo := "MemTotal:       16384256 kB\nMemFree:        1140000 kB\nMemAvailable:   3000000 kB\n"
	if err := os.WriteFile(filepath.Join(procDir, "meminfo"), []byte(memInfo), 0o600); err != nil {
		t.Fatalf("write meminfo: %v", err)
	}
	report, _, err := hw.Detect(hw.DetectOptions{ProcRoot: procDir, SysfsRoot: sysDir, Runner: fakeRunner{}})
	if err != nil {
		t.Fatalf("hardware detect failed: %v", err)
	}
	if report.CPU.LogicalCores != 2 {
		t.Fatalf("expected 2 logical cores, got %d", report.CPU.LogicalCores)
	}
	if report.CPU.PhysicalCores != 4 {
		t.Fatalf("expected 4 physical cores, got %d", report.CPU.PhysicalCores)
	}
	if report.Memory.TotalBytes != 16777478144 {
		t.Fatalf("unexpected total mem value: %d", report.Memory.TotalBytes)
	}
}

func TestSoftwareDetectionWithMockedRunner(t *testing.T) {
	report, _, err := sw.Detect(sw.DetectOptions{Runner: fakeRunner{}})
	if err != nil {
		t.Fatalf("software detect failed: %v", err)
	}
	if len(report.PackageManagers) == 0 {
		t.Fatal("expected package managers to be reported")
	}
	if len(report.Commands) == 0 {
		t.Fatal("expected commands to be reported")
	}
}

func TestUnifiedDetectionReport(t *testing.T) {
	report, err := DetectWithOptions(context.Background(), Options{OSReleasePath: filepath.Join("/etc", "os-release"), ProcRoot: "/proc", SysfsRoot: "/sys", HomeDir: "/tmp", Runner: fakeRunner{}})
	if err != nil {
		t.Fatalf("unified detection failed: %v", err)
	}
	if report.SchemaVersion != 1 {
		t.Fatalf("unexpected schema version: %d", report.SchemaVersion)
	}
	if report.CollectedAt.IsZero() {
		t.Fatal("collected_at should be set")
	}
}
