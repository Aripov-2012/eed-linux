package software

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

var systemServiceCandidates = []string{
	"bluetooth.service",
	"docker.service",
	"NetworkManager.service",
	"pipewire.service",
	"sddm.service",
	"greetd.service",
}

var userServiceCandidates = []string{
	"pipewire.service",
	"wireplumber.service",
}

func detectServices(opts DetectOptions) ([]ServiceInfo, bool, []string) {
	if opts.InitSystem != "systemd" {
		return nil, false, nil
	}
	if _, err := lookPath(opts, "systemctl"); err != nil {
		return nil, false, nil
	}

	warnings := []string{}
	systemUnits, err := listUnitFiles(opts, false)
	if err != nil {
		warnings = append(warnings, "list system service unit files: "+err.Error())
	}
	displayManager := resolveDisplayManager(opts)
	candidates := []string{"display-manager.service"}
	candidates = append(candidates, systemServiceCandidates...)
	result := make([]ServiceInfo, 0, len(candidates)+len(userServiceCandidates))
	seen := map[string]bool{}
	queryFailures := 0
	for _, unit := range candidates {
		alias := ""
		role := ""
		if unit == "display-manager.service" {
			alias = unit
			role = "display-manager"
			if displayManager != "" {
				unit = displayManager
			}
		}
		key := "system:" + unit
		if seen[key] {
			continue
		}
		seen[key] = true
		service, queryErr := inspectService(opts, unit, "system", "", role, systemUnits)
		if queryErr != nil {
			queryFailures++
		}
		service.Alias = alias
		result = append(result, service)
	}

	if os.Getenv("XDG_RUNTIME_DIR") != "" && os.Getenv("DBUS_SESSION_BUS_ADDRESS") != "" {
		userUnits, listErr := listUnitFiles(opts, true)
		if listErr != nil {
			warnings = append(warnings, "list user service unit files: "+listErr.Error())
		}
		for _, unit := range userServiceCandidates {
			service, queryErr := inspectService(opts, unit, "user", "--user", "", userUnits)
			if queryErr != nil {
				queryFailures++
			}
			result = append(result, service)
		}
	}
	if queryFailures > 0 {
		warnings = append(warnings, fmt.Sprintf("systemd could not report the state of %d service unit(s)", queryFailures))
	}
	return result, true, warnings
}

func listUnitFiles(opts DetectOptions, user bool) (map[string]bool, error) {
	args := []string{"list-unit-files", "--no-legend", "--no-pager"}
	if user {
		args = append([]string{"--user"}, args...)
	}
	out, err := runCommand(opts, "systemctl", args...)
	if err != nil {
		return nil, err
	}
	units := map[string]bool{}
	for _, line := range strings.Split(string(out), "\n") {
		fields := strings.Fields(line)
		if len(fields) > 0 && strings.HasSuffix(fields[0], ".service") {
			units[fields[0]] = true
		}
	}
	return units, nil
}

func resolveDisplayManager(opts DetectOptions) string {
	out, err := runCommand(opts, "systemctl", "show", "--property=Id", "--value", "display-manager.service")
	if err == nil {
		unit := strings.TrimSpace(string(out))
		if strings.HasSuffix(unit, ".service") && unit != "display-manager.service" {
			return unit
		}
	}

	path := opts.DefaultDisplayManagerPath
	if path == "" {
		path = "/etc/X11/default-display-manager"
	}
	data, readErr := os.ReadFile(path)
	if readErr != nil {
		if !errors.Is(readErr, os.ErrNotExist) {
			return ""
		}
		return ""
	}
	name := filepath.Base(strings.TrimSpace(string(data)))
	if name == "." || name == string(filepath.Separator) || name == "" {
		return ""
	}
	if name == "gdm3" {
		name = "gdm"
	}
	if !strings.HasSuffix(name, ".service") {
		name += ".service"
	}
	return name
}

func inspectService(opts DetectOptions, unit, scope, userFlag, role string, installedUnits map[string]bool) (ServiceInfo, error) {
	args := []string{"show", "--property=Id", "--property=LoadState", "--property=ActiveState", unit}
	if userFlag != "" {
		args = append([]string{userFlag}, args...)
	}
	out, err := runCommand(opts, "systemctl", args...)
	properties := parseShowProperties(string(out))
	name := properties["Id"]
	if name == "" {
		name = unit
	}
	loadState := properties["LoadState"]
	activeState := properties["ActiveState"]
	loaded := loadState == "loaded"
	state := activeState
	if !loaded || state == "" {
		state = "unavailable"
	}
	if state != "active" && state != "inactive" && state != "failed" &&
		state != "activating" && state != "deactivating" && state != "reloading" && state != "maintenance" {
		state = "unavailable"
	}
	installed := installedUnits[unit] || installedUnits[name] || loaded
	return ServiceInfo{
		Name: name, State: state, Available: installed || loaded,
		Installed: installed, Loaded: loaded, Scope: scope, Role: role,
	}, err
}

func parseShowProperties(output string) map[string]string {
	properties := map[string]string{}
	for _, line := range strings.Split(output, "\n") {
		key, value, ok := strings.Cut(line, "=")
		if ok {
			properties[strings.TrimSpace(key)] = strings.TrimSpace(value)
		}
	}
	return properties
}
