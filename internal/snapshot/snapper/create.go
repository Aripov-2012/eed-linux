package snapper

import (
	"fmt"
	"sort"
	"strconv"
	"strings"
	"time"

	"eed/internal/snapshot"
)

// CreateRequest is a Snapper-specific snapshot creation request.
type CreateRequest = snapshot.CreateRequest

// BuildCreateArgs creates the correct argument list for a snapshot creation request.
func BuildCreateArgs(req CreateRequest) []string {
	args := make([]string, 0, 10)
	if req.ConfigName != "" {
		args = append(args, "-c", req.ConfigName)
	}
	args = append(args, "create")
	if req.Type != "" {
		args = append(args, "--type", req.Type)
	} else {
		args = append(args, "--type", "single")
	}
	args = append(args, "--read-only")
	if req.Description != "" {
		args = append(args, "--description", req.Description)
	}
	userDataParts := []string{"eed=true"}
	if req.UserData != nil {
		keys := make([]string, 0, len(req.UserData))
		for key := range req.UserData {
			keys = append(keys, key)
		}
		sort.Strings(keys)
		for _, key := range keys {
			value := req.UserData[key]
			userDataParts = append(userDataParts, fmt.Sprintf("%s=%s", key, value))
		}
	}
	args = append(args, "--userdata", strings.Join(userDataParts, ","), "--print-number")
	return args
}

// ParseCreateOutput parses the raw snapper command output after snapshot creation.
func ParseCreateOutput(data string) (snapshot.Snapshot, error) {
	trimmed := strings.TrimSpace(data)
	if trimmed == "" {
		return snapshot.Snapshot{}, fmt.Errorf("empty snapshot creation output")
	}
	number, err := strconv.Atoi(strings.TrimSpace(trimmed))
	if err != nil {
		return snapshot.Snapshot{}, err
	}
	return snapshot.Snapshot{Number: number, Type: "single", ReadOnly: true, CreatedAt: time.Now().UTC(), EEDOwned: true}, nil
}
