package snapper

import (
	"strconv"
	"strings"
	"time"

	"eed/internal/snapshot"
)

// ParseSnapshotList parses the textual output of `snapper list`.
func ParseSnapshotList(data string) ([]snapshot.Snapshot, error) {
	var out []snapshot.Snapshot
	lines := strings.Split(data, "\n")
	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "#") || !strings.Contains(trimmed, "|") {
			continue
		}
		fields := strings.Split(trimmed, "|")
		cleaned := make([]string, 0, len(fields))
		for _, field := range fields {
			cleaned = append(cleaned, strings.TrimSpace(field))
		}
		if len(cleaned) < 3 {
			continue
		}
		if isHeader(cleaned) {
			continue
		}
		entry := snapshot.Snapshot{}
		for i, field := range cleaned {
			switch i {
			case 0:
				entry.Type = field
			case 1:
				if n, err := strconv.Atoi(field); err == nil {
					entry.Number = n
				}
			case 2:
				// pre-number column; ignored
			case 3:
				if when, err := time.Parse(time.RFC3339, field); err == nil {
					entry.CreatedAt = when
				}
			case 4:
				// user
			case 5:
				// cleanup
			case 6:
				entry.Description = field
			case 7:
				if field != "" {
					entry.UserData = map[string]string{}
					for _, part := range strings.Split(field, ",") {
						kv := strings.SplitN(part, "=", 2)
						if len(kv) == 2 {
							entry.UserData[kv[0]] = kv[1]
						}
					}
				}
			}
		}
		if entry.Number > 0 || entry.Description != "" || entry.Type != "" {
			if data, ok := entry.UserData["eed"]; ok {
				entry.EEDOwned = strings.EqualFold(strings.TrimSpace(data), "true")
			}
			out = append(out, entry)
		}
	}
	return out, nil
}

func isHeader(fields []string) bool {
	if len(fields) < 2 {
		return false
	}
	first := strings.ToLower(fields[0])
	return strings.Contains(first, "type") || strings.Contains(first, "number")
}
