package hardware

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type gpuFixtureRunner struct {
	output string
}

func (r gpuFixtureRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	if name != "lspci" || strings.Join(args, " ") != "-Dnnk" {
		return nil, errors.New("unexpected command")
	}
	return []byte(r.output), nil
}

func TestDetectGPUFromSysfsAndLSPCIForMultipleVendors(t *testing.T) {
	sysfsRoot := t.TempDir()
	devices := []struct {
		bdf, class, vendor, device, driver string
	}{
		{"0000:00:02.0", "0x030000", "0x8086", "0x46a8", "i915"},
		{"0000:01:00.0", "0x030000", "0x1002", "0x73bf", "amdgpu"},
		{"0000:02:00.0", "0x030200", "0x10de", "0x2684", "nvidia"},
	}
	for _, device := range devices {
		devicePath := filepath.Join(sysfsRoot, "devices", "pci0000:00", device.bdf)
		if err := os.MkdirAll(devicePath, 0o755); err != nil {
			t.Fatal(err)
		}
		for name, value := range map[string]string{
			"class": device.class, "vendor": device.vendor, "device": device.device,
		} {
			if err := os.WriteFile(filepath.Join(devicePath, name), []byte(value), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		driverPath := filepath.Join(devicePath, "driver")
		if err := os.Symlink(filepath.Join(sysfsRoot, "bus", "pci", "drivers", device.driver), driverPath); err != nil {
			t.Fatal(err)
		}
		busDevices := filepath.Join(sysfsRoot, "bus", "pci", "devices")
		if err := os.MkdirAll(busDevices, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(filepath.Join(sysfsRoot, "devices", "pci0000:00", device.bdf), filepath.Join(busDevices, device.bdf)); err != nil {
			t.Fatal(err)
		}
	}

	lspci := `0000:00:02.0 VGA compatible controller [0300]: Intel Corporation Alder Lake-UP3 GT2 [Iris Xe Graphics] [8086:46a8]
	Kernel driver in use: i915
0000:01:00.0 VGA compatible controller [0300]: Advanced Micro Devices, Inc. [AMD/ATI] Navi 21 [1002:73bf]
	Kernel driver in use: amdgpu
0000:02:00.0 3D controller [0302]: NVIDIA Corporation AD102 [GeForce RTX 4090] [10de:2684]
	Kernel driver in use: nvidia
`
	gpus, warnings, err := detectGPU(DetectOptions{
		SysfsRoot: sysfsRoot,
		Runner:    gpuFixtureRunner{output: lspci},
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(gpus) != len(devices) {
		t.Fatalf("expected all GPUs to be retained, got %+v", gpus)
	}
	if len(warnings) != 0 {
		t.Fatalf("unexpected GPU warnings: %v", warnings)
	}
	wantVendors := map[string]string{"0x8086": "Intel", "0x1002": "AMD", "0x10de": "NVIDIA"}
	for _, gpu := range gpus {
		wantVendor := wantVendors[gpu.VendorID]
		if wantVendor == "" || gpu.Vendor != wantVendor {
			t.Errorf("PCI vendor ID %q was not resolved correctly: %+v", gpu.VendorID, gpu)
		}
		if !gpu.DriverBound || gpu.Driver == "" {
			t.Errorf("driver binding should be reported independently from GPU detection: %+v", gpu)
		}
		if gpu.VendorID == "0x8086" && (!strings.Contains(gpu.Model, "Iris Xe") || gpu.Driver != "i915") {
			t.Errorf("expected Intel Iris Xe and i915 enrichment, got %+v", gpu)
		}
	}
}

func TestDetectGPUWithoutBoundDriverStillReportsPCIHardware(t *testing.T) {
	sysfsRoot := t.TempDir()
	devicePath := filepath.Join(sysfsRoot, "devices", "pci0000:00", "0000:00:02.0")
	if err := os.MkdirAll(devicePath, 0o755); err != nil {
		t.Fatal(err)
	}
	for name, value := range map[string]string{
		"class": "0x030000", "vendor": "0x8086", "device": "0x9a49",
	} {
		if err := os.WriteFile(filepath.Join(devicePath, name), []byte(value), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	busDevices := filepath.Join(sysfsRoot, "bus", "pci", "devices")
	if err := os.MkdirAll(busDevices, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(devicePath, filepath.Join(busDevices, "0000:00:02.0")); err != nil {
		t.Fatal(err)
	}

	gpus, _, err := detectGPU(DetectOptions{SysfsRoot: sysfsRoot, Runner: gpuFixtureRunner{}})
	if err != nil {
		t.Fatal(err)
	}
	if len(gpus) != 1 || gpus[0].VendorID != "0x8086" || gpus[0].DriverBound || gpus[0].Driver != "" {
		t.Fatalf("expected detected Intel GPU without a bound driver: %+v", gpus)
	}
}
