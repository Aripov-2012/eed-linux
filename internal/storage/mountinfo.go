package storage

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// MountEntry is a single parsed line from /proc/self/mountinfo.
type MountEntry struct {
	ID             string
	ParentID       string
	MajorMinor     string
	Root           string
	MountPoint     string
	Options        string
	OptionalFields []string
	FSType         string
	Source         string
	SuperOptions   string
}

// ParseMountInfo parses mountinfo content and returns mount entries.
func ParseMountInfo(data string) ([]MountEntry, error) {
	if strings.TrimSpace(data) == "" {
		return nil, fmt.Errorf("empty mountinfo")
	}
	var entries []MountEntry
	for _, line := range strings.Split(data, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		fields := strings.Split(line, " - ")
		if len(fields) != 2 {
			continue
		}
		prefixParts := strings.Fields(fields[0])
		if len(prefixParts) < 6 {
			continue
		}
		fsParts := strings.Fields(fields[1])
		if len(fsParts) < 3 {
			continue
		}
		entry := MountEntry{
			ID:             prefixParts[0],
			ParentID:       prefixParts[1],
			MajorMinor:     prefixParts[2],
			Root:           unescapeMountPath(prefixParts[3]),
			MountPoint:     unescapeMountPath(prefixParts[4]),
			Options:        strings.Join(prefixParts[5:], " "),
			OptionalFields: nil,
			FSType:         fsParts[0],
			Source:         unescapeMountPath(fsParts[1]),
			SuperOptions:   strings.Join(fsParts[2:], " "),
		}
		if len(prefixParts) > 6 {
			entry.OptionalFields = prefixParts[5:]
		}
		if entry.MountPoint == "" {
			entry.MountPoint = "/"
		}
		entries = append(entries, entry)
	}
	return entries, nil
}

// ParseMountInfoFile reads and parses /proc/self/mountinfo.
func ParseMountInfoFile(path string) ([]MountEntry, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	return ParseMountInfo(string(data))
}

func unescapeMountPath(value string) string {
	if value == "" {
		return value
	}
	var builder strings.Builder
	for i := 0; i < len(value); i++ {
		if value[i] == '\\' && i+3 < len(value) {
			if isOctal(value[i+1]) && isOctal(value[i+2]) && isOctal(value[i+3]) {
				parsed, err := strconv.ParseUint(value[i+1:i+4], 8, 8)
				if err == nil {
					builder.WriteByte(byte(parsed))
					i += 3
					continue
				}
			}
		}
		builder.WriteByte(value[i])
	}
	return builder.String()
}

func isOctal(ch byte) bool {
	return ch >= '0' && ch <= '7'
}
