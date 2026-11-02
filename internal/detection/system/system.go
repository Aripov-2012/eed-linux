package system

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

type CommandRunner interface {
	Run(context.Context, string, ...string) ([]byte, error)
}

type DetectOptions struct {
	OSReleasePath string
	ProcRoot      string
	SysfsRoot     string
	HomeDir       string
	MountInfoPath string
	Runner        CommandRunner
	StatFS        func(string) (fsStats, error)
}

type OSDetails struct {
	Name              string
	ID                string
	Version           string
	VersionCodename   string
	PrettyName        string
	IDLike            string
	IsSupportedTarget bool
}

type KernelDetails struct {
	Release      string
	Architecture string
}

type MountInfo struct {
	SourceDevice   string
	Root           string
	Major          int
	Minor          int
	MountPoint     string
	Filesystem     string
	Category       string
	BindMount      bool
	ReadOnly       bool
	TotalBytes     int64
	UsedBytes      int64
	FreeBytes      int64
	AvailableBytes int64
	CapacityKnown  bool
}

type BlockDevice struct {
	Name        string
	Path        string
	SizeBytes   int64
	SizeKnown   bool
	Kind        string
	Transport   string
	IsPartition bool
	Parent      string
	Major       int
	Minor       int
}

type SystemReport struct {
	OS             OSDetails
	Kernel         KernelDetails
	Architecture   string
	InitSystem     string
	BootMode       string
	SessionType    string
	Desktop        string
	DisplayManager string
	Virtualization string
	Storage        []MountInfo
	BlockDevices   []BlockDevice
}

func Detect(opts DetectOptions) (SystemReport, []string, error) {
	if opts.OSReleasePath == "" {
		opts.OSReleasePath = "/etc/os-release"
	}
	if opts.ProcRoot == "" {
		opts.ProcRoot = "/proc"
	}
	if opts.SysfsRoot == "" {
		opts.SysfsRoot = "/sys"
	}
	if opts.HomeDir == "" {
		opts.HomeDir = os.Getenv("HOME")
	}
	warnings := []string{}

	osInfo, err := detectOS(opts.OSReleasePath)
	if err != nil {
		warnings = append(warnings, err.Error())
	}
	kernelInfo, err := detectKernel(opts)
	if err != nil {
		warnings = append(warnings, err.Error())
	}
	initSystem, err := detectInitSystem(opts)
	if err != nil {
		warnings = append(warnings, err.Error())
	}
	bootMode, err := detectBootMode(opts)
	if err != nil {
		warnings = append(warnings, err.Error())
	}
	sessionType, desktop, dm, err := detectSession()
	if err != nil {
		warnings = append(warnings, err.Error())
	}
	virt, err := detectVirtualization(opts)
	if err != nil {
		warnings = append(warnings, err.Error())
	}
	storage, storageWarnings, err := detectStorage(opts)
	if err != nil {
		warnings = append(warnings, err.Error())
	}
	warnings = append(warnings, storageWarnings...)
	blockDevices, err := detectBlockDevices(opts)
	if err != nil {
		warnings = append(warnings, err.Error())
	}

	if osInfo.Name == "" {
		osInfo.Name = "Unknown"
	}
	if osInfo.ID == "" {
		osInfo.ID = "unknown"
	}
	if kernelInfo.Architecture == "" {
		kernelInfo.Architecture = runtimeArch()
	}
	if initSystem == "" {
		initSystem = "unknown"
	}
	if sessionType == "" {
		sessionType = "unknown"
	}
	if desktop == "" {
		desktop = "unknown"
	}
	if dm == "" {
		dm = "unknown"
	}
	if bootMode == "" {
		bootMode = "unknown"
	}

	return SystemReport{
		OS:             osInfo,
		Kernel:         kernelInfo,
		Architecture:   kernelInfo.Architecture,
		InitSystem:     initSystem,
		BootMode:       bootMode,
		SessionType:    sessionType,
		Desktop:        desktop,
		DisplayManager: dm,
		Virtualization: virt,
		Storage:        storage,
		BlockDevices:   blockDevices,
	}, warnings, nil
}

func runtimeArch() string {
	if arch := os.Getenv("HOSTTYPE"); arch != "" {
		return strings.TrimSpace(arch)
	}
	return "unknown"
}

func readFileIfExists(path string) ([]byte, error) {
	if _, err := os.Stat(path); err != nil {
		return nil, err
	}
	return os.ReadFile(path)
}

func findExecutableInPath(name string) string {
	if path, err := os.LookupEnv("PATH"); err == true && path != "" {
		for _, dir := range strings.Split(path, string(os.PathListSeparator)) {
			candidate := filepath.Join(dir, name)
			if info, err := os.Stat(candidate); err == nil && !info.IsDir() {
				return candidate
			}
		}
	}
	return ""
}

func commandExists(name string) bool {
	_, err := os.Stat(filepath.Join("/usr/bin", name))
	if err == nil {
		return true
	}
	_, err = os.Stat(filepath.Join("/bin", name))
	return err == nil || findExecutableInPath(name) != ""
}

func parseKeyValueLine(line string) (string, string, bool) {
	parts := strings.SplitN(line, "=", 2)
	if len(parts) != 2 {
		return "", "", false
	}
	key := strings.TrimSpace(parts[0])
	value := strings.Trim(strings.TrimSpace(parts[1]), "\"'")
	return key, value, true
}

func containsAny(text string, values ...string) bool {
	for _, value := range values {
		if strings.Contains(strings.ToLower(text), strings.ToLower(value)) {
			return true
		}
	}
	return false
}

func trimQuoted(s string) string {
	if len(s) >= 2 && ((s[0] == '\'' && s[len(s)-1] == '\'') || (s[0] == '"' && s[len(s)-1] == '"')) {
		return s[1 : len(s)-1]
	}
	return s
}

func envValue(key string) string {
	return strings.TrimSpace(os.Getenv(key))
}

func bootModeFromEFI() string {
	if _, err := os.Stat("/sys/firmware/efi"); err == nil {
		return "uefi"
	}
	return "legacy"
}

func formatWarning(msg string, err error) string {
	if err == nil {
		return msg
	}
	return fmt.Sprintf("%s: %v", msg, err)
}
