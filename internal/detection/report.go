package detection

import "time"

// Report is the unified machine-readable output of the detection engine.
type Report struct {
	SchemaVersion int            `json:"schema_version"`
	CollectedAt   time.Time      `json:"collected_at"`
	System        SystemReport   `json:"system"`
	Hardware      HardwareReport `json:"hardware"`
	Software      SoftwareReport `json:"software"`
	Warnings      []string       `json:"warnings,omitempty"`
}

type SystemReport struct {
	OS             OSDetails     `json:"os"`
	Kernel         KernelDetails `json:"kernel"`
	Architecture   string        `json:"architecture,omitempty"`
	InitSystem     string        `json:"init_system,omitempty"`
	BootMode       string        `json:"boot_mode,omitempty"`
	SessionType    string        `json:"session_type,omitempty"`
	Desktop        string        `json:"desktop,omitempty"`
	DisplayManager string        `json:"display_manager,omitempty"`
	Virtualization string        `json:"virtualization,omitempty"`
	Storage        []MountInfo   `json:"storage,omitempty"`
	BlockDevices   []BlockDevice `json:"block_devices,omitempty"`
	Warnings       []string      `json:"warnings,omitempty"`
}

type HardwareReport struct {
	CPU            CPUInfo    `json:"cpu"`
	Memory         MemoryInfo `json:"memory"`
	GPUs           []GPUInfo  `json:"gpus,omitempty"`
	DeviceType     string     `json:"device_type,omitempty"`
	BIOSVendor     string     `json:"bios_vendor,omitempty"`
	BIOSVersion    string     `json:"bios_version,omitempty"`
	BatteryPresent bool       `json:"battery_present"`
}

type SoftwareReport struct {
	PackageManagers  []PackageManagerInfo `json:"package_managers,omitempty"`
	Commands         []CommandInfo        `json:"commands,omitempty"`
	Services         []ServiceInfo        `json:"services,omitempty"`
	SystemdAvailable bool                 `json:"systemd_available"`
}

type OSDetails struct {
	Name              string `json:"name,omitempty"`
	ID                string `json:"id,omitempty"`
	Version           string `json:"version,omitempty"`
	VersionCodename   string `json:"version_codename,omitempty"`
	PrettyName        string `json:"pretty_name,omitempty"`
	IDLike            string `json:"id_like,omitempty"`
	IsSupportedTarget bool   `json:"is_supported_target"`
}

type BlockDevice struct {
	Name        string `json:"name"`
	Path        string `json:"path"`
	SizeBytes   int64  `json:"size_bytes"`
	SizeKnown   bool   `json:"size_known"`
	Kind        string `json:"kind,omitempty"`
	Transport   string `json:"transport,omitempty"`
	IsPartition bool   `json:"is_partition"`
	Parent      string `json:"parent,omitempty"`
	Major       int    `json:"major,omitempty"`
	Minor       int    `json:"minor,omitempty"`
}

type KernelDetails struct {
	Release      string `json:"release,omitempty"`
	Architecture string `json:"architecture,omitempty"`
}

type CPUInfo struct {
	Vendor        string  `json:"vendor,omitempty"`
	Model         string  `json:"model,omitempty"`
	LogicalCores  int     `json:"logical_cores"`
	PhysicalCores int     `json:"physical_cores"`
	MaxFrequency  float64 `json:"max_frequency_hz,omitempty"`
	Architecture  string  `json:"architecture,omitempty"`
}

type MemoryInfo struct {
	TotalBytes     int64 `json:"total_bytes"`
	AvailableBytes int64 `json:"available_bytes"`
	FreeBytes      int64 `json:"free_bytes"`
	UsedBytes      int64 `json:"used_bytes"`
}

type GPUInfo struct {
	Vendor      string `json:"vendor,omitempty"`
	VendorID    string `json:"vendor_id,omitempty"`
	DeviceID    string `json:"device_id,omitempty"`
	Model       string `json:"model,omitempty"`
	PCIID       string `json:"pci_id,omitempty"`
	Driver      string `json:"driver,omitempty"`
	DriverBound bool   `json:"driver_bound"`
	Class       string `json:"class,omitempty"`
}

type MountInfo struct {
	SourceDevice   string `json:"source_device,omitempty"`
	Root           string `json:"root,omitempty"`
	Major          int    `json:"major,omitempty"`
	Minor          int    `json:"minor,omitempty"`
	MountPoint     string `json:"mount_point,omitempty"`
	Filesystem     string `json:"filesystem,omitempty"`
	Category       string `json:"category,omitempty"`
	BindMount      bool   `json:"bind_mount,omitempty"`
	ReadOnly       bool   `json:"read_only,omitempty"`
	TotalBytes     int64  `json:"total_bytes"`
	UsedBytes      int64  `json:"used_bytes"`
	FreeBytes      int64  `json:"free_bytes"`
	AvailableBytes int64  `json:"available_bytes"`
	CapacityKnown  bool   `json:"capacity_known"`
}

type PackageManagerInfo struct {
	Name                         string `json:"name,omitempty"`
	Available                    bool   `json:"available"`
	Path                         string `json:"path,omitempty"`
	Detected                     bool   `json:"detected"`
	Version                      string `json:"version,omitempty"`
	HostDistribution             string `json:"host_distribution,omitempty"`
	HostNative                   bool   `json:"host_native"`
	ConfiguredTargetDistribution string `json:"configured_target_distribution,omitempty"`
	TargetPackageManager         string `json:"target_package_manager,omitempty"`
	AdapterStatus                string `json:"adapter_status,omitempty"`
	Native                       bool   `json:"native"`
	MatchesTarget                bool   `json:"matches_installation_target"`
}

type CommandInfo struct {
	Name      string `json:"name,omitempty"`
	Installed bool   `json:"installed"`
	Path      string `json:"path,omitempty"`
	Version   string `json:"version,omitempty"`
}

type ServiceInfo struct {
	Name      string `json:"name,omitempty"`
	State     string `json:"state,omitempty"`
	Available bool   `json:"available"`
	Installed bool   `json:"installed"`
	Loaded    bool   `json:"loaded"`
	Scope     string `json:"scope,omitempty"`
	Alias     string `json:"alias,omitempty"`
	Role      string `json:"role,omitempty"`
}
