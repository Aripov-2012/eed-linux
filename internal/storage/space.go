package storage

import (
	"context"
	"os/exec"
	"strings"
	"time"

	"eed/internal/storage/btrfs"
)

// SpaceReport captures read-only storage headroom and snapshot policy information.
type SpaceReport struct {
	CapacityBytes               int64    `json:"capacity_bytes"`
	UsedBytes                   int64    `json:"used_bytes"`
	AvailableBytes              int64    `json:"available_bytes"`
	BtrfsAllocatedBytes         int64    `json:"btrfs_allocated_bytes,omitempty"`
	BtrfsUnallocatedBytes       int64    `json:"btrfs_unallocated_bytes,omitempty"`
	SnapshotCount               int      `json:"snapshot_count"`
	ExclusiveSnapshotUsageBytes int64    `json:"exclusive_snapshot_usage_bytes,omitempty"`
	ExclusiveUsageKnown         bool     `json:"exclusive_usage_known"`
	DesiredRestorePoints        int      `json:"desired_restore_points"`
	ConfiguredSpaceLimitBytes   int64    `json:"configured_space_limit_bytes,omitempty"`
	ConfiguredFreeLimitBytes    int64    `json:"configured_free_limit_bytes,omitempty"`
	RetentionCount              int      `json:"retention_count,omitempty"`
	Warnings                    []string `json:"warnings,omitempty"`
}

// BuildSpaceReport derives a space-aware view from the existing storage report.
func BuildSpaceReport(report Report) SpaceReport {
	space := SpaceReport{DesiredRestorePoints: 3, ExclusiveUsageKnown: false, SnapshotCount: 0}
	for _, fs := range report.Filesystems {
		if fs.MountPoint == "/" {
			space.CapacityBytes = fs.TotalBytes
			space.UsedBytes = fs.UsedBytes
			space.AvailableBytes = fs.AvailableBytes
			break
		}
	}
	if space.CapacityBytes == 0 && len(report.Filesystems) > 0 {
		space.CapacityBytes = report.Filesystems[0].TotalBytes
		space.UsedBytes = report.Filesystems[0].UsedBytes
		space.AvailableBytes = report.Filesystems[0].AvailableBytes
	}
	if report.BtrfsUsage != nil {
		space.BtrfsAllocatedBytes = report.BtrfsUsage.DeviceAllocated
		space.BtrfsUnallocatedBytes = report.BtrfsUsage.DeviceUnallocated
		space.ExclusiveUsageKnown = report.BtrfsUsage.QuotasAvailable
		space.ExclusiveSnapshotUsageBytes = report.BtrfsUsage.DataUsageBytes + report.BtrfsUsage.MetadataUsageBytes
	}
	if space.BtrfsAllocatedBytes == 0 && report.Summary.RootFilesystem == "btrfs" {
		space.Warnings = append(space.Warnings, "filesystem is Btrfs but no quota or usage metadata was available; snapshot space is only partially known")
	}
	if space.AvailableBytes == 0 && len(report.Filesystems) > 0 {
		space.Warnings = append(space.Warnings, "filesystem free space could not be determined precisely; values may be incomplete")
	}
	return space
}

// EnrichWithBtrfsUsage adds optional Btrfs output when the root filesystem is Btrfs and the tool is installed.
func EnrichWithBtrfsUsage(report *Report) {
	EnrichWithBtrfsUsageUsing(report, btrfs.ExecRunner{}, exec.LookPath)
}

func EnrichWithBtrfsUsageUsing(report *Report, runner btrfs.CommandRunner, lookPath func(string) (string, error)) {
	if report == nil {
		return
	}
	if runner == nil {
		runner = btrfs.ExecRunner{}
	}
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	for _, fs := range report.Filesystems {
		if strings.EqualFold(fs.Filesystem, "btrfs") && fs.MountPoint == "/" {
			ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
			defer cancel()
			tool, toolErr := lookPath("btrfs")
			if toolErr != nil {
				report.Warnings = append(report.Warnings, "optional Btrfs usage inspection unavailable at "+fs.MountPoint+": "+toolErr.Error())
				report.Warnings = append(report.Warnings, "Btrfs subvolume layout could not be inspected at "+fs.MountPoint+": "+toolErr.Error())
			} else {
				usage, err := btrfs.AnalyzeUsageWithRunner(ctx, tool, fs.MountPoint, runner)
				if err == nil {
					report.BtrfsUsage = &usage
				} else {
					report.Warnings = append(report.Warnings, "optional Btrfs usage inspection unavailable at "+fs.MountPoint+": "+err.Error())
				}
				subvolumes, subvolumeErr := btrfs.ListSubvolumesWithRunner(ctx, tool, fs.MountPoint, runner)
				if subvolumeErr != nil {
					report.Warnings = append(report.Warnings, "Btrfs subvolume layout could not be inspected at "+fs.MountPoint+": "+subvolumeErr.Error())
				} else {
					report.BtrfsLayoutInspected = true
					report.BtrfsSubvolumes = subvolumes
				}
			}
			for _, mounted := range report.Filesystems {
				if strings.EqualFold(mounted.Filesystem, "btrfs") && mounted.MountPoint != "/" {
					report.BtrfsSeparateMounts = append(report.BtrfsSeparateMounts, mounted.MountPoint)
				}
			}
			return
		}
	}
}
