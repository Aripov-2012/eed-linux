package storage

import (
	"time"

	"eed/internal/storage/btrfs"
)

// Report describes the current storage layout in a read-only, machine-friendly format.
type Report struct {
	SchemaVersion        string                `json:"schema_version"`
	CollectedAt          time.Time             `json:"collected_at"`
	Filesystems          []Filesystem          `json:"filesystems"`
	SkippedMounts        []SkippedMountGroup   `json:"skipped_mounts,omitempty"`
	Summary              Summary               `json:"summary"`
	BtrfsUsage           *btrfs.UsageReport    `json:"btrfs_usage,omitempty"`
	BtrfsSubvolumes      []btrfs.SubvolumeInfo `json:"btrfs_subvolumes,omitempty"`
	BtrfsLayoutInspected bool                  `json:"btrfs_layout_inspected"`
	BtrfsSeparateMounts  []string              `json:"btrfs_separate_mounts,omitempty"`
	Warnings             []string              `json:"warnings,omitempty"`
}

// SkippedMountGroup summarizes optional mounts excluded from capacity checks.
type SkippedMountGroup struct {
	Category    string   `json:"category"`
	Count       int      `json:"count"`
	MountPoints []string `json:"mount_points,omitempty"`
	Reasons     []string `json:"reasons,omitempty"`
}

// Summary captures the most important root and snapshot-related storage facts.
type Summary struct {
	RootFilesystem             string `json:"root_filesystem,omitempty"`
	RootMountPoint             string `json:"root_mountpoint,omitempty"`
	HomeFilesystem             string `json:"home_filesystem,omitempty"`
	HomeMountPoint             string `json:"home_mountpoint,omitempty"`
	BootFilesystem             string `json:"boot_filesystem,omitempty"`
	BootMountPoint             string `json:"boot_mountpoint,omitempty"`
	EFISystemPartition         string `json:"efi_system_partition,omitempty"`
	SnapshotBackendCandidate   string `json:"snapshot_backend_candidate,omitempty"`
	EstimatedDownloadBytes     int64  `json:"estimated_download_bytes,omitempty"`
	EstimatedInstallationBytes int64  `json:"estimated_installation_bytes,omitempty"`
	TemporaryInstallationBytes int64  `json:"temporary_installation_bytes,omitempty"`
	MinimumFreeReserveBytes    int64  `json:"minimum_free_reserve_bytes,omitempty"`
	ReadOnly                   bool   `json:"read_only"`
}

// Filesystem describes one mounted filesystem and its capacity metrics.
type Filesystem struct {
	MountPoint       string   `json:"mount_point,omitempty"`
	SourceDevice     string   `json:"source_device,omitempty"`
	Filesystem       string   `json:"filesystem,omitempty"`
	TotalBytes       int64    `json:"total_bytes"`
	UsedBytes        int64    `json:"used_bytes"`
	FreeBytes        int64    `json:"free_bytes"`
	AvailableBytes   int64    `json:"available_bytes"`
	ReadOnly         bool     `json:"read_only"`
	DeviceMajor      int      `json:"device_major,omitempty"`
	DeviceMinor      int      `json:"device_minor,omitempty"`
	MountOptions     []string `json:"mount_options,omitempty"`
	LVMLogicalVolume bool     `json:"lvm_logical_volume"`
	LVMThin          bool     `json:"lvm_thin"`
	LVMStatus        string   `json:"lvm_status,omitempty"`
}
