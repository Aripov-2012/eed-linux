package system

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

func detectOS(path string) (OSDetails, error) {
	if path == "" {
		path = "/etc/os-release"
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return OSDetails{}, fmt.Errorf("read %s: %w", path, err)
	}
	values := map[string]string{}
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		key, value, ok := parseKeyValueLine(line)
		if !ok {
			continue
		}
		value, err := parseOSReleaseValue(value)
		if err != nil {
			return OSDetails{}, fmt.Errorf("parse %s field %s: %w", path, key, err)
		}
		values[key] = value
	}
	info := OSDetails{
		Name:            values["NAME"],
		ID:              values["ID"],
		Version:         values["VERSION_ID"],
		VersionCodename: values["VERSION_CODENAME"],
		PrettyName:      values["PRETTY_NAME"],
		IDLike:          values["ID_LIKE"],
	}
	if info.Name == "" {
		info.Name = values["DISTRIB_ID"]
	}
	if info.Version == "" {
		info.Version = values["VERSION"]
	}
	if info.PrettyName == "" {
		info.PrettyName = info.Name
	}
	if info.ID == "" {
		info.ID = strings.ToLower(strings.TrimSpace(info.Name))
	}
	if info.ID == "arch" || info.IDLike == "arch" || strings.Contains(strings.ToLower(info.Name), "arch") {
		info.IsSupportedTarget = true
	}
	return info, nil
}

func parseOSReleaseValue(value string) (string, error) {
	value = strings.TrimSpace(value)
	if len(value) >= 2 && value[0] == '"' && value[len(value)-1] == '"' {
		decoded, err := strconv.Unquote(value)
		if err != nil {
			return "", err
		}
		return decoded, nil
	}
	if len(value) >= 2 && value[0] == '\'' && value[len(value)-1] == '\'' {
		return value[1 : len(value)-1], nil
	}
	return value, nil
}
