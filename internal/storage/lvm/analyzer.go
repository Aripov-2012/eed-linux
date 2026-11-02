package lvm

import (
	"strconv"
	"strings"
)

// VolumeGroup captures an LVM volume group.
type VolumeGroup struct {
	Name      string `json:"name,omitempty"`
	SizeBytes int64  `json:"size_bytes"`
	FreeBytes int64  `json:"free_bytes"`
}

// LogicalVolume captures an LVM logical volume.
type LogicalVolume struct {
	Name      string `json:"name,omitempty"`
	Path      string `json:"path,omitempty"`
	VGName    string `json:"vg_name,omitempty"`
	SizeBytes int64  `json:"size_bytes"`
}

// ThinPool captures LVM thin-pool details.
type ThinPool struct {
	Name           string `json:"name,omitempty"`
	VGName         string `json:"vg_name,omitempty"`
	DataUsageBytes int64  `json:"data_usage_bytes"`
	MetadataBytes  int64  `json:"metadata_usage_bytes"`
}

// ParseVolumeGroups parses `vgs --units b --noheadings -o vg_name,vg_size,vg_free` output.
func ParseVolumeGroups(data string) []VolumeGroup {
	var groups []VolumeGroup
	for _, line := range strings.Split(data, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 3 {
			continue
		}
		size, _ := parseSize(fields[1])
		free, _ := parseSize(fields[2])
		groups = append(groups, VolumeGroup{Name: fields[0], SizeBytes: size, FreeBytes: free})
	}
	return groups
}

// ParseLogicalVolumes parses `lvs --units b --noheadings -o lv_name,lv_path,vg_name,lv_size` output.
func ParseLogicalVolumes(data string) []LogicalVolume {
	var values []LogicalVolume
	for _, line := range strings.Split(data, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		size, err := parseSize(fields[3])
		if err != nil {
			continue
		}
		values = append(values, LogicalVolume{Name: fields[0], Path: fields[1], VGName: fields[2], SizeBytes: size})
	}
	return values
}

func parseSize(value string) (int64, error) {
	val := strings.TrimSpace(value)
	if val == "" {
		return 0, nil
	}
	upper := strings.ToUpper(val)
	for _, suffix := range []string{"P", "T", "G", "M", "K", "B"} {
		if strings.HasSuffix(upper, suffix) {
			base := strings.TrimSpace(strings.TrimSuffix(val, suffix))
			base = strings.TrimSpace(strings.TrimSuffix(base, strings.ToLower(suffix)))
			parsed, err := strconv.ParseFloat(base, 64)
			if err != nil {
				return 0, err
			}
			switch suffix {
			case "B":
				return int64(parsed), nil
			case "K":
				return int64(parsed * 1024), nil
			case "M":
				return int64(parsed * 1024 * 1024), nil
			case "G":
				return int64(parsed * 1024 * 1024 * 1024), nil
			case "T":
				return int64(parsed * 1024 * 1024 * 1024 * 1024), nil
			case "P":
				return int64(parsed * 1024 * 1024 * 1024 * 1024 * 1024), nil
			default:
				return int64(parsed), nil
			}
		}
	}
	parsed, err := strconv.ParseFloat(val, 64)
	if err != nil {
		return 0, err
	}
	return int64(parsed), nil
}
