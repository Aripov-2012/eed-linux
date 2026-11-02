package hardware

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"strconv"
	"strings"
	"time"
)

type CommandRunner interface {
	Run(context.Context, string, ...string) ([]byte, error)
}

type DetectOptions struct {
	ProcRoot  string
	SysfsRoot string
	DevRoot   string
	Runner    CommandRunner
}

type CPUInfo struct {
	Vendor        string
	Model         string
	LogicalCores  int
	PhysicalCores int
	MaxFrequency  float64
	Architecture  string
}

type MemoryInfo struct {
	TotalBytes     int64
	AvailableBytes int64
	FreeBytes      int64
	UsedBytes      int64
}

type GPUInfo struct {
	Vendor      string
	VendorID    string
	DeviceID    string
	Model       string
	PCIID       string
	Driver      string
	DriverBound bool
	Class       string
	Detected    bool
	Virtual     bool
	Physical    bool
}

type GraphicsInfo struct {
	DRMDeviceAvailable           bool
	RenderNodeAvailable          bool
	OpenGLRenderer               string
	HardwareAcceleratedRendering bool
	SoftwareRenderingFallback    bool
	AccelerationStatus           string
	RendererStatus               string
	RendererError                string
}

type HardwareReport struct {
	CPU            CPUInfo
	Memory         MemoryInfo
	GPUDetected    bool
	GPUs           []GPUInfo
	Graphics       GraphicsInfo
	DeviceType     string
	BIOSVendor     string
	BIOSVersion    string
	BatteryPresent bool
}

func Detect(opts DetectOptions) (HardwareReport, []string, error) {
	if opts.ProcRoot == "" {
		opts.ProcRoot = "/proc"
	}
	if opts.SysfsRoot == "" {
		opts.SysfsRoot = "/sys"
	}
	if opts.DevRoot == "" {
		opts.DevRoot = "/dev"
	}

	warnings := []string{}
	cpu, cpuWarnings, err := detectCPU(opts)
	if err != nil {
		warnings = append(warnings, err.Error())
	}
	warnings = append(warnings, cpuWarnings...)

	mem, memWarnings, err := detectMemory(opts)
	if err != nil {
		warnings = append(warnings, err.Error())
	} else {
		warnings = append(warnings, memWarnings...)
	}

	gpus, gpuWarnings, err := detectGPU(opts)
	if err != nil {
		warnings = append(warnings, err.Error())
	} else {
		warnings = append(warnings, gpuWarnings...)
	}
	graphics := detectGraphics(opts)

	deviceType, deviceWarnings, err := detectDeviceType(opts)
	if err != nil {
		warnings = append(warnings, err.Error())
	} else {
		warnings = append(warnings, deviceWarnings...)
	}

	return HardwareReport{
		CPU:            cpu,
		Memory:         mem,
		GPUDetected:    len(gpus) > 0,
		GPUs:           gpus,
		Graphics:       graphics,
		DeviceType:     deviceType,
		BIOSVendor:     readDMIValue(filepath.Join(opts.SysfsRoot, "class", "dmi", "id", "bios_vendor")),
		BIOSVersion:    readDMIValue(filepath.Join(opts.SysfsRoot, "class", "dmi", "id", "bios_version")),
		BatteryPresent: batteryExists(opts.SysfsRoot),
	}, warnings, nil
}

func detectCPU(opts DetectOptions) (CPUInfo, []string, error) {
	info := CPUInfo{Architecture: runtimeArch()}
	filePath := filepath.Join(opts.ProcRoot, "cpuinfo")
	data, err := os.ReadFile(filePath)
	if err != nil {
		return info, nil, fmt.Errorf("read %s: %w", filePath, err)
	}

	var model strings.Builder
	var vendor strings.Builder
	logical := 0
	physical := 0
	seenCoreIDs := map[string]struct{}{}
	var maxFreq float64

	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := scanner.Text()
		if strings.HasPrefix(line, "model name") || strings.HasPrefix(line, "Hardware") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				if model.Len() == 0 {
					model.WriteString(strings.TrimSpace(parts[1]))
				}
			}
		}
		if strings.HasPrefix(line, "vendor_id") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 && vendor.Len() == 0 {
				vendor.WriteString(strings.TrimSpace(parts[1]))
			}
		}
		if strings.HasPrefix(line, "cpu MHz") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				if freq, err := strconv.ParseFloat(strings.TrimSpace(parts[1]), 64); err == nil {
					maxFreq = freq * 1000000
				}
			}
		}
		if strings.HasPrefix(line, "cpu cores") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				if cores, err := strconv.Atoi(strings.TrimSpace(parts[1])); err == nil {
					physical = cores
				}
			}
		}
		if strings.HasPrefix(line, "core id") {
			parts := strings.SplitN(line, ":", 2)
			if len(parts) == 2 {
				seenCoreIDs[strings.TrimSpace(parts[1])] = struct{}{}
			}
		}
		if strings.HasPrefix(line, "processor") {
			logical++
		}
	}
	if logical == 0 {
		logical = 1
	}
	if physical == 0 && len(seenCoreIDs) > 0 {
		physical = len(seenCoreIDs)
	}
	if physical == 0 && logical > 0 {
		physical = logical
	}
	if vendor.Len() == 0 {
		vendor.WriteString("unknown")
	}
	if model.Len() == 0 {
		model.WriteString("unknown")
	}

	info.Vendor = strings.TrimSpace(vendor.String())
	info.Model = strings.TrimSpace(model.String())
	info.LogicalCores = logical
	info.PhysicalCores = physical
	info.MaxFrequency = maxFreq
	return info, nil, nil
}

func detectMemory(opts DetectOptions) (MemoryInfo, []string, error) {
	mem := MemoryInfo{}
	path := filepath.Join(opts.ProcRoot, "meminfo")
	data, err := os.ReadFile(path)
	if err != nil {
		return mem, nil, fmt.Errorf("read %s: %w", path, err)
	}
	kv := map[string]int64{}
	for _, line := range strings.Split(string(data), "\n") {
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		name := strings.TrimSuffix(parts[0], ":")
		val, err := strconv.ParseInt(parts[1], 10, 64)
		if err != nil {
			continue
		}
		if len(parts) > 2 && strings.EqualFold(parts[2], "kB") {
			val *= 1024
		}
		kv[name] = val
	}
	mem.TotalBytes = kv["MemTotal"]
	mem.AvailableBytes = kv["MemAvailable"]
	mem.FreeBytes = kv["MemFree"]
	mem.UsedBytes = mem.TotalBytes - mem.AvailableBytes
	if mem.UsedBytes < 0 {
		mem.UsedBytes = 0
	}
	return mem, nil, nil
}

func detectGPU(opts DetectOptions) ([]GPUInfo, []string, error) {
	deviceRoot := filepath.Join(opts.SysfsRoot, "bus", "pci", "devices")
	entries, err := os.ReadDir(deviceRoot)
	var gpus []GPUInfo
	if err != nil {
		return detectGPUFromLSPCI(opts, fmt.Errorf("read PCI devices: %w", err))
	}
	classReadFailures := 0
	for _, entry := range entries {
		path := filepath.Join(deviceRoot, entry.Name())
		class, readErr := os.ReadFile(filepath.Join(path, "class"))
		if readErr != nil {
			classReadFailures++
			continue
		}
		classText := strings.TrimSpace(string(class))
		if !isDisplayController(classText) {
			continue
		}
		vendor, vendorErr := os.ReadFile(filepath.Join(path, "vendor"))
		device, deviceErr := os.ReadFile(filepath.Join(path, "device"))
		vendorText := normalizePCIID(string(vendor))
		deviceText := normalizePCIID(string(device))
		if vendorErr != nil {
			vendorText = ""
		}
		if deviceErr != nil {
			deviceText = ""
		}
		driver := readLink(filepath.Join(path, "driver"))
		gpus = append(gpus, GPUInfo{
			Vendor:      resolveVendorName(vendorText),
			VendorID:    vendorText,
			DeviceID:    deviceText,
			Model:       resolveGPUName(vendorText, deviceText),
			PCIID:       entry.Name(),
			Driver:      driver,
			DriverBound: driver != "",
			Class:       classText,
			Detected:    true,
			Virtual:     isVirtualGPU(vendorText, deviceText),
			Physical:    isPhysicalGPU(vendorText) && !isVirtualGPU(vendorText, deviceText),
		})
	}
	var lspciErr error
	if opts.Runner != nil {
		enriched, enrichErr := readLSPCI(opts.Runner)
		if enrichErr == nil {
			gpus = mergeGPUInfo(gpus, enriched)
		} else {
			lspciErr = enrichErr
		}
	}
	if len(gpus) > 0 {
		return gpus, nil, nil
	}
	if classReadFailures == 0 {
		return []GPUInfo{}, nil, nil
	}
	if lspciErr != nil {
		return nil, []string{fmt.Sprintf("%d PCI device class entries could not be read from sysfs; optional lspci enrichment failed: %v", classReadFailures, lspciErr)}, nil
	}
	return []GPUInfo{}, nil, nil
}

var (
	lspciBDFPattern   = regexp.MustCompile(`^[[:xdigit:]]{4}:[[:xdigit:]]{2}:[[:xdigit:]]{2}\.[[:xdigit:]]`)
	lspciIDPattern    = regexp.MustCompile(`\[([[:xdigit:]]{4}):([[:xdigit:]]{4})\]`)
	lspciClassPattern = regexp.MustCompile(`\[03[[:xdigit:]]{2}\]`)
)

func detectGPUFromLSPCI(opts DetectOptions, sysfsErr error) ([]GPUInfo, []string, error) {
	if opts.Runner == nil {
		return nil, nil, sysfsErr
	}
	gpus, err := readLSPCI(opts.Runner)
	if err != nil {
		return nil, []string{"GPU details were not available from sysfs (" + sysfsErr.Error() + ") or optional lspci enrichment (" + err.Error() + ")"}, nil
	}
	return gpus, nil, nil
}

func readLSPCI(runner CommandRunner) ([]GPUInfo, error) {
	data, err := runner.Run(context.Background(), "lspci", "-Dnnk")
	if err != nil {
		return nil, err
	}
	var gpus []GPUInfo
	var current *GPUInfo
	for _, line := range strings.Split(string(data), "\n") {
		trimmed := strings.TrimSpace(line)
		if lspciBDFPattern.MatchString(trimmed) {
			current = nil
			if !lspciClassPattern.MatchString(trimmed) {
				continue
			}
			bdf := strings.Fields(trimmed)[0]
			ids := lspciIDPattern.FindStringSubmatch(trimmed)
			vendorID, deviceID := "", ""
			if len(ids) == 3 {
				vendorID = normalizePCIID(ids[1])
				deviceID = normalizePCIID(ids[2])
			}
			model := trimmed
			if _, after, ok := strings.Cut(trimmed, ": "); ok {
				model = strings.TrimSpace(after)
			}
			gpus = append(gpus, GPUInfo{
				Vendor:   resolveVendorName(vendorID),
				VendorID: vendorID,
				DeviceID: deviceID,
				Model:    model,
				PCIID:    bdf,
				Class:    "lspci",
				Detected: true,
				Virtual:  isVirtualGPU(vendorID, deviceID),
				Physical: isPhysicalGPU(vendorID) && !isVirtualGPU(vendorID, deviceID),
			})
			current = &gpus[len(gpus)-1]
			continue
		}
		if current != nil && strings.HasPrefix(trimmed, "Kernel driver in use:") {
			current.Driver = strings.TrimSpace(strings.TrimPrefix(trimmed, "Kernel driver in use:"))
			current.DriverBound = current.Driver != ""
		}
	}
	return gpus, nil
}

func mergeGPUInfo(sysfs, lspci []GPUInfo) []GPUInfo {
	byBDF := make(map[string]int, len(sysfs))
	for i := range sysfs {
		byBDF[sysfs[i].PCIID] = i
	}
	for _, enriched := range lspci {
		index, exists := byBDF[enriched.PCIID]
		if !exists {
			sysfs = append(sysfs, enriched)
			byBDF[enriched.PCIID] = len(sysfs) - 1
			continue
		}
		gpu := &sysfs[index]
		if gpu.Vendor == "" || gpu.Vendor == "unknown" {
			gpu.Vendor = enriched.Vendor
		}
		if gpu.VendorID == "" {
			gpu.VendorID = enriched.VendorID
		}
		if gpu.DeviceID == "" {
			gpu.DeviceID = enriched.DeviceID
		}
		if gpu.Model == "" || gpu.Model == "unknown" ||
			strings.EqualFold(gpu.Model, strings.TrimPrefix(gpu.DeviceID, "0x")) {
			gpu.Model = enriched.Model
		}
		if gpu.Driver == "" {
			gpu.Driver = enriched.Driver
			gpu.DriverBound = enriched.DriverBound
		}
		gpu.Detected = true
		gpu.Virtual = gpu.Virtual || enriched.Virtual
		gpu.Physical = isPhysicalGPU(gpu.VendorID) && !gpu.Virtual
	}
	return sysfs
}

func isDisplayController(class string) bool {
	class = strings.ToLower(strings.TrimSpace(class))
	if strings.HasPrefix(class, "0x") {
		class = strings.TrimPrefix(class, "0x")
	}
	return len(class) >= 4 && strings.HasPrefix(class, "03")
}

func normalizePCIID(value string) string {
	value = strings.ToLower(strings.TrimSpace(value))
	value = strings.TrimPrefix(value, "0x")
	if value == "" {
		return ""
	}
	return "0x" + value
}

func detectDeviceType(opts DetectOptions) (string, []string, error) {
	if batteryExists(opts.SysfsRoot) {
		return "laptop", nil, nil
	}
	if _, err := os.Stat(filepath.Join(opts.SysfsRoot, "firmware", "efi")); err == nil {
		return "desktop", nil, nil
	}
	if _, err := os.Stat(filepath.Join(opts.SysfsRoot, "class", "dmi", "id", "product_name")); err == nil {
		return "desktop", nil, nil
	}
	return "unknown", nil, nil
}

func batteryExists(sysfsRoot string) bool {
	entries, err := os.ReadDir(filepath.Join(sysfsRoot, "class", "power_supply"))
	if err != nil {
		return false
	}
	for _, entry := range entries {
		if strings.Contains(strings.ToLower(entry.Name()), "battery") || strings.Contains(strings.ToLower(entry.Name()), "bat") {
			return true
		}
	}
	return false
}

func runtimeArch() string {
	if arch := runtime.GOARCH; arch != "" {
		return arch
	}
	return "unknown"
}

func readDMIValue(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}

func readLink(path string) string {
	link, err := os.Readlink(path)
	if err != nil {
		return ""
	}
	return filepath.Base(link)
}

func resolveVendorName(vendor string) string {
	value := strings.TrimPrefix(normalizePCIID(vendor), "0x")
	switch value {
	case "8086":
		return "Intel"
	case "1002", "1022":
		return "AMD"
	case "10de":
		return "NVIDIA"
	case "1af4":
		return "Virtio"
	case "1b36":
		return "QEMU"
	case "15ad":
		return "VMware"
	case "80ee":
		return "VirtualBox"
	case "1414":
		return "Microsoft"
	}
	if strings.TrimSpace(vendor) == "" {
		return "unknown"
	}
	return value
}

func isVirtualGPU(vendorID, deviceID string) bool {
	vendor := strings.TrimPrefix(normalizePCIID(vendorID), "0x")
	device := strings.TrimPrefix(normalizePCIID(deviceID), "0x")
	switch vendor {
	case "1af4":
		return device == "1050" || device == "1051" || device == "1052"
	case "1b36":
		return device == "0100"
	case "15ad":
		return device == "0405" || device == "0406"
	case "80ee":
		return device == "beef"
	case "1414":
		return device == "5353"
	default:
		return false
	}
}

func isPhysicalGPU(vendorID string) bool {
	switch strings.TrimPrefix(normalizePCIID(vendorID), "0x") {
	case "8086", "1002", "1022", "10de":
		return true
	default:
		return false
	}
}

func detectGraphics(opts DetectOptions) GraphicsInfo {
	graphics := GraphicsInfo{RendererStatus: "not_checked", AccelerationStatus: "unknown"}
	drmRoot := filepath.Join(opts.SysfsRoot, "class", "drm")
	entries, err := os.ReadDir(drmRoot)
	if err == nil {
		for _, entry := range entries {
			if strings.HasPrefix(entry.Name(), "card") && !strings.Contains(entry.Name(), "-") {
				if _, statErr := os.Stat(filepath.Join(opts.DevRoot, "dri", entry.Name())); statErr == nil {
					graphics.DRMDeviceAvailable = true
				}
			}
			if strings.HasPrefix(entry.Name(), "renderD") {
				if _, sysfsErr := os.Stat(filepath.Join(drmRoot, entry.Name())); sysfsErr == nil {
					if _, devErr := os.Stat(filepath.Join(opts.DevRoot, "dri", entry.Name())); devErr == nil {
						graphics.RenderNodeAvailable = true
					}
				}
			}
		}
	}
	if opts.Runner != nil {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		output, runErr := opts.Runner.Run(ctx, "glxinfo", "-B")
		cancel()
		if runErr == nil {
			graphics.OpenGLRenderer = parseOpenGLRenderer(string(output))
		} else {
			graphics.RendererError = "glxinfo: " + runErr.Error()
		}
		if graphics.OpenGLRenderer == "" {
			ctx, cancel = context.WithTimeout(context.Background(), 5*time.Second)
			output, runErr = opts.Runner.Run(ctx, "eglinfo", "--display", "surfaceless")
			cancel()
			if runErr == nil {
				graphics.OpenGLRenderer = parseOpenGLRenderer(string(output))
			} else {
				detail := "eglinfo: " + runErr.Error()
				if graphics.RendererError != "" {
					graphics.RendererError += "; "
				}
				graphics.RendererError += detail
			}
		}
		if graphics.OpenGLRenderer != "" {
			graphics.RendererStatus = "detected"
			graphics.RendererError = ""
		} else {
			graphics.RendererStatus = "unavailable"
			if graphics.RendererError == "" {
				graphics.RendererError = "OpenGL renderer string was absent from glxinfo and eglinfo output"
			}
		}
	}
	renderer := strings.ToLower(graphics.OpenGLRenderer)
	if renderer != "" {
		graphics.SoftwareRenderingFallback = strings.Contains(renderer, "llvmpipe") ||
			strings.Contains(renderer, "softpipe") ||
			strings.Contains(renderer, "software rasterizer") ||
			strings.Contains(renderer, "swrast")
		graphics.HardwareAcceleratedRendering = !graphics.SoftwareRenderingFallback
		if graphics.SoftwareRenderingFallback {
			graphics.AccelerationStatus = "software_fallback"
		} else {
			graphics.AccelerationStatus = "available"
		}
	} else {
		graphics.AccelerationStatus = "unknown"
	}
	return graphics
}

func parseOpenGLRenderer(output string) string {
	for _, line := range strings.Split(output, "\n") {
		key, value, ok := strings.Cut(line, ":")
		key = strings.TrimSpace(key)
		if ok && (strings.EqualFold(key, "OpenGL renderer string") || strings.EqualFold(key, "GL_RENDERER")) {
			return strings.TrimSpace(value)
		}
	}
	return ""
}

func resolveGPUName(vendor, device string) string {
	vendor = strings.TrimPrefix(normalizePCIID(vendor), "0x")
	device = strings.TrimPrefix(normalizePCIID(device), "0x")
	if vendor == "" && device == "" {
		return "unknown"
	}
	return "unknown"
}
