package btrfs

import (
	"context"
	"os/exec"
	"strconv"
	"strings"
)

// ListSubvolumes runs `btrfs subvolume list` for the target path and returns subvolume metadata.
func ListSubvolumes(ctx context.Context, target string) ([]SubvolumeInfo, error) {
	tool, err := exec.LookPath("btrfs")
	if err != nil {
		return nil, err
	}
	cmd := exec.CommandContext(ctx, tool, "subvolume", "list", "-o", target)
	data, err := cmd.Output()
	if err != nil {
		return nil, err
	}
	return ParseSubvolumeList(string(data)), nil
}

// ParseSubvolumeList parses `btrfs subvolume list` output.
func ParseSubvolumeList(data string) []SubvolumeInfo {
	var result []SubvolumeInfo
	for _, line := range strings.Split(data, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || !strings.HasPrefix(trimmed, "ID ") {
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) < 8 {
			continue
		}
		id, err := strconv.Atoi(fields[1])
		if err != nil {
			continue
		}
		info := SubvolumeInfo{ID: id}
		for i := 0; i < len(fields); i++ {
			switch fields[i] {
			case "parent":
				if i+1 < len(fields) {
					if parentID, err := strconv.Atoi(fields[i+1]); err == nil {
						info.ParentID = parentID
					}
				}
			case "top-level":
				if i+1 < len(fields) {
					if topLevel, err := strconv.Atoi(fields[i+1]); err == nil {
						info.TopLevel = topLevel
					}
				}
			case "path":
				if i+1 < len(fields) {
					info.Path = strings.Join(fields[i+1:], " ")
				}
			case "ro":
				info.Readonly = true
			case "rw":
				info.Readonly = false
			}
		}
		result = append(result, info)
	}
	return result
}
