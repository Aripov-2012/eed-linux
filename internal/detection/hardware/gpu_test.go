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

type graphicsFixtureRunner struct {
	output string
}

func (r graphicsFixtureRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	if name == "glxinfo" && strings.Join(args, " ") == "-B" {
		return []byte(r.output), nil
	}
	return nil, errors.New("unexpected command")
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
		if !gpu.Detected || gpu.Virtual || !gpu.Physical {
			t.Errorf("physical GPU status should be separate from vendor IDs: %+v", gpu)
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

func TestDetectGPURecognizesVirtioAndQXLWithoutClaimingAcceleration(t *testing.T) {
	for _, test := range []struct {
		name, vendor, device, model string
	}{
		{"virtio", "0x1af4", "0x1050", "Virtio"},
		{"qxl", "0x1b36", "0x0100", "QEMU"},
	} {
		t.Run(test.name, func(t *testing.T) {
			sysfsRoot := t.TempDir()
			devicePath := filepath.Join(sysfsRoot, "bus", "pci", "devices", "0000:00:02.0")
			if err := os.MkdirAll(devicePath, 0o755); err != nil {
				t.Fatal(err)
			}
			for name, value := range map[string]string{
				"class": "0x030000", "vendor": test.vendor, "device": test.device,
			} {
				if err := os.WriteFile(filepath.Join(devicePath, name), []byte(value), 0o600); err != nil {
					t.Fatal(err)
				}
			}
			gpus, _, err := detectGPU(DetectOptions{SysfsRoot: sysfsRoot})
			if err != nil {
				t.Fatal(err)
			}
			if len(gpus) != 1 || !gpus[0].Detected || !gpus[0].Virtual || gpus[0].Physical {
				t.Fatalf("expected detected virtual %s GPU, got %+v", test.name, gpus)
			}
			if gpus[0].Vendor != test.model {
				t.Fatalf("expected vendor label %q, got %+v", test.model, gpus[0])
			}
			graphics := detectGraphics(DetectOptions{SysfsRoot: sysfsRoot})
			if graphics.HardwareAcceleratedRendering {
				t.Fatalf("a virtual GPU must not imply 3D acceleration: %+v", graphics)
			}
		})
	}
}

func TestDetectGraphicsDistinguishesSoftwareRenderingAndMissingDevices(t *testing.T) {
	sysfsRoot := t.TempDir()
	devRoot := filepath.Join(sysfsRoot, "dev")
	renderer := detectGraphics(DetectOptions{
		SysfsRoot: sysfsRoot,
		DevRoot:   devRoot,
		Runner:    graphicsFixtureRunner{output: "OpenGL renderer string: llvmpipe (LLVM 18.1.0, 256 bits)\n"},
	})
	if renderer.OpenGLRenderer == "" || !renderer.SoftwareRenderingFallback ||
		renderer.HardwareAcceleratedRendering || renderer.DRMDeviceAvailable || renderer.RenderNodeAvailable ||
		renderer.AccelerationStatus != "software_fallback" {
		t.Fatalf("software renderer and missing DRM devices should be reported independently: %+v", renderer)
	}

	drm := filepath.Join(sysfsRoot, "class", "drm")
	if err := os.MkdirAll(drm, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(drm, "card0"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(drm, "renderD128"), 0o755); err != nil {
		t.Fatal(err)
	}
	devices := filepath.Join(devRoot, "dri")
	if err := os.MkdirAll(devices, 0o755); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"card0", "renderD128"} {
		if err := os.WriteFile(filepath.Join(devices, name), nil, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	renderer = detectGraphics(DetectOptions{
		SysfsRoot: sysfsRoot,
		DevRoot:   devRoot,
		Runner:    graphicsFixtureRunner{output: "OpenGL renderer string: AMD Radeon RX 7900\n"},
	})
	if !renderer.DRMDeviceAvailable || !renderer.RenderNodeAvailable ||
		!renderer.HardwareAcceleratedRendering || renderer.SoftwareRenderingFallback ||
		renderer.AccelerationStatus != "available" {
		t.Fatalf("DRM nodes and hardware renderer should be detected independently: %+v", renderer)
	}
}

func TestDetectGraphicsReportsAccelerationUnavailableWithoutRenderer(t *testing.T) {
	graphics := detectGraphics(DetectOptions{SysfsRoot: t.TempDir(), Runner: graphicsFixtureRunner{}})
	if graphics.HardwareAcceleratedRendering || graphics.SoftwareRenderingFallback ||
		graphics.OpenGLRenderer != "" || graphics.AccelerationStatus != "unknown" {
		t.Fatalf("missing renderer must remain unknown rather than being inferred: %+v", graphics)
	}
}

func TestDetectGraphicsMissingRenderNode(t *testing.T) {
	sysfsRoot := t.TempDir()
	devRoot := filepath.Join(sysfsRoot, "dev")
	drm := filepath.Join(sysfsRoot, "class", "drm")
	if err := os.MkdirAll(drm, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Mkdir(filepath.Join(drm, "card0"), 0o755); err != nil {
		t.Fatal(err)
	}
	devices := filepath.Join(devRoot, "dri")
	if err := os.MkdirAll(devices, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(devices, "card0"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	graphics := detectGraphics(DetectOptions{
		SysfsRoot: sysfsRoot,
		DevRoot:   devRoot,
		Runner:    graphicsFixtureRunner{output: "OpenGL renderer string: AMD Radeon RX 7900\n"},
	})
	if !graphics.DRMDeviceAvailable {
		t.Fatalf("DRM card device should be detected: %+v", graphics)
	}
	if graphics.RenderNodeAvailable {
		t.Fatalf("render node must be reported missing when renderD* does not exist: %+v", graphics)
	}
}

func TestDetectGPUMissingGPUDevice(t *testing.T) {
	sysfsRoot := t.TempDir()
	busDevices := filepath.Join(sysfsRoot, "bus", "pci", "devices")
	if err := os.MkdirAll(busDevices, 0o755); err != nil {
		t.Fatal(err)
	}
	report, _, err := Detect(DetectOptions{
		SysfsRoot: sysfsRoot,
		ProcRoot:  t.TempDir(),
		Runner:    gpuFixtureRunner{output: ""},
	})
	if err != nil {
		t.Fatal(err)
	}
	if report.GPUDetected || len(report.GPUs) != 0 {
		t.Fatalf("expected no GPU detected on headless or missing-GPU system: %+v", report)
	}
}

func TestDetectGraphicsHardwareAccelerationUnavailable(t *testing.T) {
	sysfsRoot := t.TempDir()
	devRoot := filepath.Join(sysfsRoot, "dev")
	graphics := detectGraphics(DetectOptions{
		SysfsRoot: sysfsRoot,
		DevRoot:   devRoot,
		Runner:    graphicsFixtureRunner{output: "OpenGL renderer string: softpipe\n"},
	})
	if graphics.HardwareAcceleratedRendering || !graphics.SoftwareRenderingFallback ||
		graphics.AccelerationStatus != "software_fallback" {
		t.Fatalf("software renderer softpipe must report acceleration unavailable: %+v", graphics)
	}
}
