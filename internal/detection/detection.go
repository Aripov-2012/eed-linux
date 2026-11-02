package detection

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"time"

	hw "eed/internal/detection/hardware"
	sw "eed/internal/detection/software"
	sys "eed/internal/detection/system"
)

// Options allows dependency injection and testing for the detection engine.
type Options struct {
	OSReleasePath string
	ProcRoot      string
	SysfsRoot     string
	HomeDir       string
	Runner        CommandRunner
}

// CommandRunner executes commands strictly without a shell.
type CommandRunner interface {
	Run(context.Context, string, ...string) ([]byte, error)
}

type execRunner struct{}

func (execRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	if name == "" {
		return nil, fmt.Errorf("missing executable name")
	}
	return exec.CommandContext(ctx, name, args...).Output()
}

func appDefaultOptions() Options {
	return Options{
		OSReleasePath: "/etc/os-release",
		ProcRoot:      "/proc",
		SysfsRoot:     "/sys",
		HomeDir:       os.Getenv("HOME"),
		Runner:        execRunner{},
	}
}

func Detect(ctx context.Context) (Report, error) {
	return DetectWithOptions(ctx, appDefaultOptions())
}

func DetectWithOptions(ctx context.Context, opts Options) (Report, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if opts.Runner == nil {
		opts.Runner = execRunner{}
	}
	if opts.OSReleasePath == "" {
		opts.OSReleasePath = "/etc/os-release"
	}
	if opts.ProcRoot == "" {
		opts.ProcRoot = "/proc"
	}
	if opts.SysfsRoot == "" {
		opts.SysfsRoot = "/sys"
	}

	hwReport, hwWarnings, err := hw.Detect(hw.DetectOptions{ProcRoot: opts.ProcRoot, SysfsRoot: opts.SysfsRoot, Runner: opts.Runner})
	if err != nil {
		return Report{}, fmt.Errorf("hardware detection: %w", err)
	}

	systemReport, sysWarnings, err := sys.Detect(sys.DetectOptions{OSReleasePath: opts.OSReleasePath, ProcRoot: opts.ProcRoot, SysfsRoot: opts.SysfsRoot, HomeDir: opts.HomeDir, Runner: opts.Runner})
	if err != nil {
		return Report{}, fmt.Errorf("system detection: %w", err)
	}

	softwareReport, swWarnings, err := sw.Detect(sw.DetectOptions{
		Runner: opts.Runner, OSID: systemReport.OS.ID, InitSystem: systemReport.InitSystem,
	})
	if err != nil {
		return Report{}, fmt.Errorf("software detection: %w", err)
	}
	if softwareReport.DisplayManager != "" {
		systemReport.DisplayManager = softwareReport.DisplayManager
	}

	report := Report{
		SchemaVersion: 1,
		CollectedAt:   time.Now().UTC(),
		System: SystemReport{
			OS:             toOSDetails(systemReport.OS),
			Kernel:         toKernelDetails(systemReport.Kernel),
			Architecture:   systemReport.Architecture,
			InitSystem:     systemReport.InitSystem,
			BootMode:       systemReport.BootMode,
			SessionType:    systemReport.SessionType,
			Desktop:        systemReport.Desktop,
			DisplayManager: systemReport.DisplayManager,
			Virtualization: systemReport.Virtualization,
			Storage:        toStorage(systemReport.Storage),
			BlockDevices:   toBlockDevices(systemReport.BlockDevices),
			Warnings:       append([]string{}, sysWarnings...),
		},
		Hardware: HardwareReport{
			CPU:            toCPUInfo(hwReport.CPU),
			Memory:         toMemoryInfo(hwReport.Memory),
			GPUDetected:    hwReport.GPUDetected,
			GPUs:           toGPUInfo(hwReport.GPUs),
			Graphics:       toGraphicsInfo(hwReport.Graphics),
			DeviceType:     hwReport.DeviceType,
			BIOSVendor:     hwReport.BIOSVendor,
			BIOSVersion:    hwReport.BIOSVersion,
			BatteryPresent: hwReport.BatteryPresent,
		},
		Software: SoftwareReport{
			PackageManagers:  toPackageManagers(softwareReport.PackageManagers),
			Commands:         toCommands(softwareReport.Commands),
			Services:         toServices(softwareReport.Services),
			SystemdAvailable: softwareReport.SystemdAvailable,
		},
		Warnings: append([]string{}, hwWarnings...),
	}
	report.Warnings = append(report.Warnings, sysWarnings...)
	report.Warnings = append(report.Warnings, swWarnings...)
	return report, nil
}

func (r Report) MarshalJSON() ([]byte, error) {
	type alias Report
	return json.Marshal(alias(r))
}

func toOSDetails(in sys.OSDetails) OSDetails {
	return OSDetails{
		Name:              in.Name,
		ID:                in.ID,
		Version:           in.Version,
		VersionCodename:   in.VersionCodename,
		PrettyName:        in.PrettyName,
		IDLike:            in.IDLike,
		IsSupportedTarget: in.IsSupportedTarget,
	}
}

func toBlockDevices(in []sys.BlockDevice) []BlockDevice {
	out := make([]BlockDevice, 0, len(in))
	for _, item := range in {
		out = append(out, BlockDevice{
			Name: item.Name, Path: item.Path, SizeBytes: item.SizeBytes,
			SizeKnown: item.SizeKnown, Kind: item.Kind, Transport: item.Transport,
			IsPartition: item.IsPartition, Parent: item.Parent, Major: item.Major, Minor: item.Minor,
		})
	}
	return out
}

func toKernelDetails(in sys.KernelDetails) KernelDetails {
	return KernelDetails{Release: in.Release, Architecture: in.Architecture}
}

func toStorage(in []sys.MountInfo) []MountInfo {
	out := make([]MountInfo, 0, len(in))
	for _, item := range in {
		out = append(out, MountInfo{
			SourceDevice: item.SourceDevice, Root: item.Root, Major: item.Major, Minor: item.Minor,
			MountPoint: item.MountPoint, Filesystem: item.Filesystem, Category: item.Category,
			BindMount: item.BindMount, ReadOnly: item.ReadOnly,
			TotalBytes: item.TotalBytes, UsedBytes: item.UsedBytes,
			FreeBytes: item.FreeBytes, AvailableBytes: item.AvailableBytes,
			CapacityKnown: item.CapacityKnown,
		})
	}
	return out
}

func toCPUInfo(in hw.CPUInfo) CPUInfo {
	return CPUInfo{
		Vendor:        in.Vendor,
		Model:         in.Model,
		LogicalCores:  in.LogicalCores,
		PhysicalCores: in.PhysicalCores,
		MaxFrequency:  in.MaxFrequency,
		Architecture:  in.Architecture,
	}
}

func toMemoryInfo(in hw.MemoryInfo) MemoryInfo {
	return MemoryInfo{TotalBytes: in.TotalBytes, AvailableBytes: in.AvailableBytes, FreeBytes: in.FreeBytes, UsedBytes: in.UsedBytes}
}

func toGPUInfo(in []hw.GPUInfo) []GPUInfo {
	out := make([]GPUInfo, 0, len(in))
	for _, item := range in {
		out = append(out, GPUInfo{
			Vendor: item.Vendor, VendorID: item.VendorID, DeviceID: item.DeviceID, Model: item.Model,
			PCIID: item.PCIID, Driver: item.Driver, DriverBound: item.DriverBound, Class: item.Class,
			Detected: item.Detected, Virtual: item.Virtual, Physical: item.Physical,
		})
	}
	return out
}

func toGraphicsInfo(in hw.GraphicsInfo) GraphicsInfo {
	return GraphicsInfo{
		DRMDeviceAvailable: in.DRMDeviceAvailable, RenderNodeAvailable: in.RenderNodeAvailable,
		OpenGLRenderer: in.OpenGLRenderer, HardwareAcceleratedRendering: in.HardwareAcceleratedRendering,
		SoftwareRenderingFallback: in.SoftwareRenderingFallback, AccelerationStatus: in.AccelerationStatus,
		RendererStatus: in.RendererStatus, RendererError: in.RendererError,
	}
}

func toPackageManagers(in []sw.PackageManagerInfo) []PackageManagerInfo {
	out := make([]PackageManagerInfo, 0, len(in))
	for _, item := range in {
		out = append(out, PackageManagerInfo{
			Name: item.Name, Available: item.Available, Path: item.Path, Detected: item.Detected,
			Version: item.Version, HostDistribution: item.HostDistribution, HostNative: item.HostNative,
			ConfiguredTargetDistribution: item.ConfiguredTargetDistribution,
			TargetPackageManager:         item.TargetPackageManager,
			AdapterStatus:                item.AdapterStatus,
			Native:                       item.Native, MatchesTarget: item.MatchesTarget,
		})
	}
	return out
}

func toCommands(in []sw.CommandInfo) []CommandInfo {
	out := make([]CommandInfo, 0, len(in))
	for _, item := range in {
		out = append(out, CommandInfo{Name: item.Name, Installed: item.Installed, Path: item.Path, Version: item.Version})
	}
	return out
}

func toServices(in []sw.ServiceInfo) []ServiceInfo {
	out := make([]ServiceInfo, 0, len(in))
	for _, item := range in {
		out = append(out, ServiceInfo{
			Name: item.Name, State: item.State, Available: item.Available,
			Installed: item.Installed, Loaded: item.Loaded, Scope: item.Scope,
			Alias: item.Alias, Role: item.Role,
		})
	}
	return out
}
