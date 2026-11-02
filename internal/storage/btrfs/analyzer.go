package btrfs

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

// UsageReport summarizes Btrfs filesystem usage status.
type UsageReport struct {
	FilesystemUUID     string   `json:"filesystem_uuid,omitempty"`
	DeviceSizeBytes    int64    `json:"device_size_bytes"`
	DeviceAllocated    int64    `json:"device_allocated_bytes"`
	DeviceUnallocated  int64    `json:"device_unallocated_bytes"`
	DeviceMissingBytes int64    `json:"device_missing_bytes"`
	DataUsageBytes     int64    `json:"data_usage_bytes"`
	MetadataUsageBytes int64    `json:"metadata_usage_bytes"`
	DeviceCount        int      `json:"device_count"`
	QuotasAvailable    bool     `json:"quotas_available"`
	Warnings           []string `json:"warnings,omitempty"`
}

// SubvolumeInfo captures a Btrfs subvolume path and metadata.
type SubvolumeInfo struct {
	ID       int    `json:"id"`
	Path     string `json:"path,omitempty"`
	ParentID int    `json:"parent_id,omitempty"`
	Readonly bool   `json:"read_only"`
	Mounted  bool   `json:"mounted"`
	TopLevel int    `json:"top_level,omitempty"`
}

// AnalyzeUsage invokes btrfs filesystem usage when available and converts the output to a report.
func AnalyzeUsage(ctx context.Context, target string) (UsageReport, error) {
	tool, err := exec.LookPath("btrfs")
	if err != nil {
		return UsageReport{}, err
	}
	return AnalyzeUsageWithRunner(ctx, tool, target, ExecRunner{})
}

func AnalyzeUsageWithRunner(ctx context.Context, tool, target string, runner CommandRunner) (UsageReport, error) {
	if tool == "" || target == "" || runner == nil {
		return UsageReport{}, fmt.Errorf("btrfs executable, target, and command runner are required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	data, err := runner.Run(ctx, tool, "filesystem", "usage", "--raw", target)
	if err != nil {
		return UsageReport{}, err
	}
	return ParseUsageOutput(string(data)), nil
}

// ParseUsageOutput converts raw `btrfs filesystem usage` output to a typed report.
func ParseUsageOutput(data string) UsageReport {
	report := UsageReport{QuotasAvailable: false}
	for _, line := range strings.Split(data, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") {
			continue
		}
		parts := strings.Fields(trimmed)
		if len(parts) < 2 {
			continue
		}
		switch {
		case strings.HasPrefix(trimmed, "Device size"):
			report.DeviceSizeBytes = parseSizeValue(trimmed[len("Device size"):])
		case strings.HasPrefix(trimmed, "Device allocated"):
			report.DeviceAllocated = parseSizeValue(trimmed[len("Device allocated"):])
		case strings.HasPrefix(trimmed, "Device unallocated"):
			report.DeviceUnallocated = parseSizeValue(trimmed[len("Device unallocated"):])
		case strings.HasPrefix(trimmed, "Device missing"):
			report.DeviceMissingBytes = parseSizeValue(trimmed[len("Device missing"):])
		case strings.HasPrefix(trimmed, "Data,"):
			report.DataUsageBytes = parseSizeValue(strings.TrimSpace(strings.TrimPrefix(trimmed, "Data,")))
		case strings.HasPrefix(trimmed, "Metadata,"):
			report.MetadataUsageBytes = parseSizeValue(strings.TrimSpace(strings.TrimPrefix(trimmed, "Metadata,")))
		case strings.HasPrefix(trimmed, "Filesystem UUID"):
			report.FilesystemUUID = strings.TrimSpace(strings.TrimPrefix(trimmed, "Filesystem UUID"))
		case strings.Contains(trimmed, "qgroup"):
			report.QuotasAvailable = true
		}
	}
	return report
}

func parseSizeValue(value string) int64 {
	v := strings.TrimSpace(value)
	v = strings.TrimPrefix(v, ":")
	v = strings.TrimSuffix(v, ":")
	v = strings.TrimSpace(v)
	for _, unit := range []string{"PiB", "TiB", "GiB", "MiB", "KiB", "B"} {
		if strings.HasSuffix(strings.ToUpper(v), strings.ToUpper(unit)) {
			base := strings.TrimSuffix(v, unit)
			base = strings.TrimSuffix(base, strings.ToLower(unit))
			base = strings.TrimSpace(base)
			parsed, err := strconv.ParseFloat(base, 64)
			if err != nil {
				return 0
			}
			switch strings.ToUpper(unit) {
			case "KIB":
				return int64(parsed * 1024)
			case "MIB":
				return int64(parsed * 1024 * 1024)
			case "GIB":
				return int64(parsed * 1024 * 1024 * 1024)
			case "TIB":
				return int64(parsed * 1024 * 1024 * 1024 * 1024)
			case "PIB":
				return int64(parsed * 1024 * 1024 * 1024 * 1024 * 1024)
			default:
				return int64(parsed)
			}
		}
	}
	parsed, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return 0
	}
	return int64(parsed)
}
