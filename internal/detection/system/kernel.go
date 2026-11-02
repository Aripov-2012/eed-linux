package system

import (
	"fmt"
	"os"
	"runtime"
	"strings"
)

func detectKernel(opts DetectOptions) (KernelDetails, error) {
	release := "unknown"
	architecture := runtime.GOARCH

	if data, err := os.ReadFile(filepathJoin(opts.ProcRoot, "version")); err == nil {
		line := strings.SplitN(string(data), "\n", 2)[0]
		parts := strings.Fields(line)
		if len(parts) >= 3 {
			release = parts[2]
		}
	}
	if release == "unknown" {
		if data, err := os.ReadFile(filepathJoin(opts.ProcRoot, "cmdline")); err == nil {
			if strings.Contains(string(data), "BOOT_IMAGE") {
				release = "unknown"
			}
		}
	}
	if architecture == "" {
		architecture = runtime.GOARCH
	}
	return KernelDetails{Release: release, Architecture: architecture}, nil
}

func filepathJoin(parts ...string) string {
	path := ""
	for _, part := range parts {
		if part == "" {
			continue
		}
		if path == "" {
			path = part
			continue
		}
		path = path + "/" + part
	}
	return path
}

func readKernelRelease() (string, error) {
	data, err := os.ReadFile("/proc/version")
	if err != nil {
		return "", fmt.Errorf("read /proc/version: %w", err)
	}
	line := strings.SplitN(string(data), "\n", 2)[0]
	parts := strings.Fields(line)
	if len(parts) >= 3 {
		return parts[2], nil
	}
	return "unknown", nil
}
