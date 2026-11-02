package lvm

import (
	"strconv"
	"strings"
)

// ThinPoolUsage describes LVM thin-pool data and metadata usage.
type ThinPoolUsage struct {
	Name          string `json:"name,omitempty"`
	VGName        string `json:"vg_name,omitempty"`
	DataUsage     int64  `json:"data_usage_bytes"`
	MetadataUsage int64  `json:"metadata_usage_bytes"`
}

// ParseThinPoolUsage parses `lvs --units b --noheadings -o lv_name,vg_name,data_percent,metadata_percent` output.
func ParseThinPoolUsage(data string) []ThinPoolUsage {
	var out []ThinPoolUsage
	for _, line := range strings.Split(data, "\n") {
		fields := strings.Fields(line)
		if len(fields) < 4 {
			continue
		}
		dataPercent, err := strconv.ParseFloat(fields[2], 64)
		if err != nil {
			continue
		}
		metaPercent, err := strconv.ParseFloat(fields[3], 64)
		if err != nil {
			continue
		}
		out = append(out, ThinPoolUsage{
			Name:          fields[0],
			VGName:        fields[1],
			DataUsage:     int64(dataPercent * 1024 * 1024),
			MetadataUsage: int64(metaPercent * 1024 * 1024),
		})
	}
	return out
}
