package btrfs

import (
	"context"
	"fmt"
	"os/exec"
	"strconv"
	"strings"
)

type CommandRunner interface {
	Run(context.Context, string, ...string) ([]byte, error)
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	return exec.CommandContext(ctx, name, args...).Output()
}

// ListSubvolumes runs `btrfs subvolume list` for the target path and returns subvolume metadata.
func ListSubvolumes(ctx context.Context, target string) ([]SubvolumeInfo, error) {
	tool, err := exec.LookPath("btrfs")
	if err != nil {
		return nil, err
	}
	return ListSubvolumesWithRunner(ctx, tool, target, ExecRunner{})
}

func ListSubvolumesWithRunner(ctx context.Context, tool, target string, runner CommandRunner) ([]SubvolumeInfo, error) {
	if tool == "" || target == "" || runner == nil {
		return nil, fmt.Errorf("btrfs executable, target, and command runner are required")
	}
	if ctx == nil {
		ctx = context.Background()
	}
	data, err := runner.Run(ctx, tool, "subvolume", "list", "-o", target)
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
