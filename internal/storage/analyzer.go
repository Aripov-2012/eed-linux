package storage

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"syscall"
	"time"
)

// Analyze inspects the host filesystems in read-only mode and returns a storage report.
func Analyze() (Report, error) {
	return analyze("/proc/self/mountinfo", statPath, EnrichWithBtrfsUsage)
}

func analyze(mountInfoPath string, statFilesystem func(string) (fsStats, error), enrich func(*Report)) (Report, error) {
	report := Report{
		SchemaVersion: "1",
		CollectedAt:   time.Now().UTC(),
		Warnings:      []string{},
	}

	entries, err := ParseMountInfoFile(mountInfoPath)
	if err != nil {
		report.Warnings = append(report.Warnings, fmt.Sprintf("required mount metadata unavailable: read %s: %v", mountInfoPath, err))
	}
	if len(entries) == 0 {
		entries = []MountEntry{{MountPoint: "/", Source: "unknown", FSType: "unknown"}}
	}

	seen := map[string]struct{}{}
	skippedMounts := make(map[string]skippedMountDetails)
	var targetMountErr error
	for _, entry := range entries {
		if entry.MountPoint == "" {
			continue
		}
		key := entry.Source + "|" + entry.FSType + "|" + entry.MountPoint
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}

		if category := skippedMountCategory(entry); category != "" {
			addSkippedMount(skippedMounts, category, entry.MountPoint, "")
			continue
		}
		fs, warn := filesystemFromMountWithStat(entry, statFilesystem)
		if warn != nil {
			if entry.MountPoint == "/" {
				targetMountErr = warn
			} else {
				addSkippedMount(skippedMounts, "inaccessible_optional", entry.MountPoint, warn.Error())
			}
			continue
		}
		report.Filesystems = append(report.Filesystems, fs)
	}
	report.SkippedMounts = summarizeSkippedMounts(skippedMounts)
	if warning := inaccessibleMountWarning(report.SkippedMounts); warning != "" {
		report.Warnings = append(report.Warnings, warning)
	}

	if len(report.Filesystems) == 0 {
		for _, candidate := range []string{"/", os.Getenv("HOME")} {
			if candidate == "" {
				continue
			}
			fs, err := fallbackFilesystemWithStat(candidate, statFilesystem)
			if err == nil {
				report.Filesystems = append(report.Filesystems, fs)
			} else {
				report.Warnings = append(report.Warnings, fmt.Sprintf("required filesystem capacity unavailable at %s: %v", candidate, err))
			}
		}
	}

	if len(report.Filesystems) == 0 {
		return report, fmt.Errorf("unable to determine storage information: %s", strings.Join(uniqueStorageWarnings(report.Warnings), "; "))
	}

	report.Summary = summarize(report.Filesystems)
	if targetMountErr != nil {
		report.Summary.RootFilesystem = "unknown"
		report.Summary.RootMountPoint = "/"
	}
	report.Summary.SnapshotBackendCandidate = snapshotBackendCandidate(report.Filesystems)
	if enrich != nil {
		enrich(&report)
	}
	report.Warnings = uniqueStorageWarnings(report.Warnings)
	if report.Summary.RootMountPoint == "" {
		report.Summary.RootMountPoint = "/"
	}
	if report.Summary.HomeMountPoint == "" && os.Getenv("HOME") != "" {
		report.Summary.HomeMountPoint = os.Getenv("HOME")
	}
	if targetMountErr != nil {
		return report, fmt.Errorf("required target filesystem capacity unavailable at /: %w", targetMountErr)
	}

	return report, nil
}

type skippedMountDetails struct {
	mountPoints []string
	reasons     []string
}

func skippedMountCategory(entry MountEntry) string {
	fstype := strings.ToLower(strings.TrimSpace(entry.FSType))
	source := strings.ToLower(strings.TrimSpace(entry.Source))
	if entry.MountPoint != "/" && (fstype == "overlay" || source == "overlay") {
		return "container_overlay"
	}
	switch fstype {
	case "autofs", "bpf", "cgroup", "cgroup2", "configfs", "debugfs", "devpts", "devtmpfs",
		"fuse.portal", "fusectl", "hugetlbfs", "mqueue", "proc", "pstore", "ramfs", "securityfs",
		"sysfs", "tmpfs", "tracefs":
		return "pseudo_filesystem"
	}
	if source == "portal" || strings.HasSuffix(strings.TrimRight(entry.MountPoint, "/"), "/doc") {
		return "pseudo_filesystem"
	}
	return ""
}

func addSkippedMount(groups map[string]skippedMountDetails, category, mountPoint, reason string) {
	group := groups[category]
	group.mountPoints = append(group.mountPoints, mountPoint)
	if reason != "" {
		group.reasons = append(group.reasons, reason)
	}
	groups[category] = group
}

func summarizeSkippedMounts(groups map[string]skippedMountDetails) []SkippedMountGroup {
	order := []string{"pseudo_filesystem", "container_overlay", "inaccessible_optional"}
	result := make([]SkippedMountGroup, 0, len(groups))
	for _, category := range order {
		details, ok := groups[category]
		if !ok {
			continue
		}
		group := SkippedMountGroup{
			Category:    category,
			Count:       len(details.mountPoints),
			MountPoints: sampleMountPoints(details.mountPoints),
			Reasons:     sampleMountPoints(details.reasons),
		}
		result = append(result, group)
	}
	return result
}

func sampleMountPoints(values []string) []string {
	seen := make(map[string]bool, len(values))
	samples := make([]string, 0, 3)
	for _, value := range values {
		if seen[value] {
			continue
		}
		seen[value] = true
		samples = append(samples, value)
		if len(samples) == 3 {
			break
		}
	}
	return samples
}

func inaccessibleMountWarning(groups []SkippedMountGroup) string {
	for _, group := range groups {
		if group.Category != "inaccessible_optional" {
			continue
		}
		details := strings.Join(group.MountPoints, ", ")
		if group.Count > len(group.MountPoints) {
			details += fmt.Sprintf(" (and %d more)", group.Count-len(group.MountPoints))
		}
		reason := strings.Join(group.Reasons, ", ")
		return fmt.Sprintf("skipped %d inaccessible optional mountpoints (%s): %s", group.Count, details, reason)
	}
	return ""
}

func fallbackFilesystem(path string) (Filesystem, error) {
	return fallbackFilesystemWithStat(path, statPath)
}

func fallbackFilesystemWithStat(path string, statFilesystem func(string) (fsStats, error)) (Filesystem, error) {
	path = filepath.Clean(path)
	info, err := statFilesystem(path)
	if err != nil {
		return Filesystem{}, err
	}
	return Filesystem{
		MountPoint:     path,
		SourceDevice:   "unknown",
		Filesystem:     detectFilesystem(path),
		TotalBytes:     info.TotalBytes,
		UsedBytes:      info.UsedBytes,
		FreeBytes:      info.FreeBytes,
		AvailableBytes: info.AvailableBytes,
	}, nil
}

func filesystemFromMount(entry MountEntry) (Filesystem, error) {
	return filesystemFromMountWithStat(entry, statPath)
}

func filesystemFromMountWithStat(entry MountEntry, statFilesystem func(string) (fsStats, error)) (Filesystem, error) {
	path := filepath.Clean(entry.MountPoint)
	if path == "." {
		path = "/"
	}
	stats, err := statFilesystem(path)
	if err != nil {
		return Filesystem{}, err
	}
	options := strings.Split(entry.Options, ",")
	readOnly := false
	for _, opt := range options {
		if opt == "ro" || strings.TrimSpace(opt) == "ro" {
			readOnly = true
			break
		}
	}
	major, minor := parseMajorMinor(entry.MajorMinor)
	return Filesystem{
		MountPoint:     entry.MountPoint,
		SourceDevice:   entry.Source,
		Filesystem:     strings.TrimSpace(entry.FSType),
		TotalBytes:     stats.TotalBytes,
		UsedBytes:      stats.UsedBytes,
		FreeBytes:      stats.FreeBytes,
		AvailableBytes: stats.AvailableBytes,
		ReadOnly:       readOnly,
		DeviceMajor:    major,
		DeviceMinor:    minor,
		MountOptions:   options,
	}, nil
}

func uniqueStorageWarnings(warnings []string) []string {
	seen := make(map[string]struct{}, len(warnings))
	unique := make([]string, 0, len(warnings))
	for _, warning := range warnings {
		if _, exists := seen[warning]; exists {
			continue
		}
		seen[warning] = struct{}{}
		unique = append(unique, warning)
	}
	return unique
}

func summarize(filesystems []Filesystem) Summary {
	summary := Summary{ReadOnly: false}
	for _, fs := range filesystems {
		if fs.MountPoint == "/" {
			summary.RootFilesystem = fs.Filesystem
			summary.RootMountPoint = fs.MountPoint
			summary.ReadOnly = summary.ReadOnly || fs.ReadOnly
		}
		if strings.HasPrefix(fs.MountPoint, "/home") || fs.MountPoint == os.Getenv("HOME") {
			summary.HomeFilesystem = fs.Filesystem
			summary.HomeMountPoint = fs.MountPoint
		}
		if fs.MountPoint == "/boot" || strings.HasPrefix(fs.MountPoint, "/boot/") {
			summary.BootFilesystem = fs.Filesystem
			summary.BootMountPoint = fs.MountPoint
		}
		if fs.MountPoint == "/boot/efi" || strings.HasPrefix(fs.MountPoint, "/boot/efi") {
			summary.EFISystemPartition = fs.SourceDevice
		}
	}
	if summary.RootFilesystem == "" && len(filesystems) > 0 {
		summary.RootFilesystem = filesystems[0].Filesystem
		summary.RootMountPoint = filesystems[0].MountPoint
	}
	if summary.RootFilesystem == "" {
		summary.RootFilesystem = "unknown"
	}
	if summary.HomeFilesystem == "" {
		summary.HomeFilesystem = "unknown"
	}
	return summary
}

func snapshotBackendCandidate(filesystems []Filesystem) string {
	for _, fs := range filesystems {
		if fs.MountPoint == "/" && strings.EqualFold(fs.Filesystem, "btrfs") {
			return "btrfs_snapper"
		}
	}
	return "unknown"
}

func statPath(path string) (fsStats, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return fsStats{}, err
	}
	blockSize := int64(stat.Bsize)
	return fsStats{
		TotalBytes:     int64(stat.Blocks) * blockSize,
		FreeBytes:      int64(stat.Bfree) * blockSize,
		AvailableBytes: int64(stat.Bavail) * blockSize,
		UsedBytes:      (int64(stat.Blocks) - int64(stat.Bfree)) * blockSize,
	}, nil
}

type fsStats struct {
	TotalBytes     int64
	UsedBytes      int64
	FreeBytes      int64
	AvailableBytes int64
}

func detectFilesystem(path string) string {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return "unknown"
	}
	switch stat.Type {
	case 0x9123683E: // BTRFS_SUPER_MAGIC
		return "btrfs"
	case 0xEF53: // EXT4_SUPER_MAGIC
		return "ext4"
	case 0x58465342: // XFS_SUPER_MAGIC
		return "xfs"
	case 0xF2F52010: // ZFS_SUPER_MAGIC
		return "zfs"
	default:
		return "unknown"
	}
}

func parseMajorMinor(value string) (int, int) {
	if value == "" {
		return 0, 0
	}
	parts := strings.SplitN(value, ":", 2)
	if len(parts) != 2 {
		return 0, 0
	}
	major := 0
	minor := 0
	fmt.Sscanf(parts[0], "%d", &major)
	fmt.Sscanf(parts[1], "%d", &minor)
	return major, minor
}
