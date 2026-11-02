package snapper

import (
	"strings"
)

// Config describes a validated Snapper configuration.
type Config struct {
	Name        string `json:"name,omitempty"`
	Type        string `json:"type,omitempty"`
	Subvolume   string `json:"subvolume,omitempty"`
	Description string `json:"description,omitempty"`
}

// ParseConfigs parses snapper list-configs output.
func ParseConfigs(data string) []Config {
	var out []Config
	for _, line := range strings.Split(data, "\n") {
		trimmed := strings.TrimSpace(line)
		if trimmed == "" || strings.HasPrefix(trimmed, "Config") || strings.HasPrefix(trimmed, "#") {
			continue
		}
		fields := strings.Fields(trimmed)
		if len(fields) == 0 {
			continue
		}
		config := Config{Name: fields[0]}
		if len(fields) > 1 {
			config.Type = fields[1]
		}
		if len(fields) > 2 {
			config.Subvolume = strings.Join(fields[2:], " ")
		}
		out = append(out, config)
	}
	return out
}
