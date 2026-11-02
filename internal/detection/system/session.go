package system

import (
	"context"
	"fmt"
	"os"
	"strings"
)

func detectSession() (string, string, string, error) {
	sessionType := strings.TrimSpace(os.Getenv("XDG_SESSION_TYPE"))
	if sessionType == "" {
		sessionType = strings.TrimSpace(os.Getenv("WAYLAND_DISPLAY"))
		if sessionType != "" {
			sessionType = "wayland"
		}
	}
	if sessionType == "" {
		sessionType = strings.TrimSpace(os.Getenv("DISPLAY"))
		if sessionType != "" {
			sessionType = "x11"
		}
	}
	if sessionType == "" {
		sessionType = "unknown"
	}

	desktop := strings.TrimSpace(os.Getenv("XDG_CURRENT_DESKTOP"))
	if desktop == "" {
		desktop = strings.TrimSpace(os.Getenv("DESKTOP_SESSION"))
	}
	if desktop == "" {
		desktop = strings.TrimSpace(os.Getenv("GNOME_DESKTOP_SESSION_ID"))
	}
	if desktop == "" {
		desktop = "unknown"
	}

	dm := strings.TrimSpace(os.Getenv("XDG_SESSION_DESKTOP"))
	if dm == "" {
		dm = strings.TrimSpace(os.Getenv("DISPLAYMANAGER"))
	}
	if dm == "" {
		dm = "unknown"
	}
	return sessionType, desktop, dm, nil
}

func detectInitSystem(opts DetectOptions) (string, error) {
	procFile := filepathJoin(opts.ProcRoot, "1", "comm")
	data, err := os.ReadFile(procFile)
	if err != nil {
		return "unknown", fmt.Errorf("read %s: %w", procFile, err)
	}
	initName := strings.TrimSpace(string(data))
	if initName == "" {
		return "unknown", nil
	}
	if strings.Contains(initName, "systemd") {
		return "systemd", nil
	}
	return initName, nil
}

func detectBootMode(opts DetectOptions) (string, error) {
	if _, err := os.Stat(filepathJoin(opts.SysfsRoot, "firmware", "efi")); err == nil {
		return "uefi", nil
	}
	return "legacy", nil
}

func detectVirtualization(opts DetectOptions) (string, error) {
	if opts.Runner != nil {
		if data, err := opts.Runner.Run(contextBackground(), "systemd-detect-virt"); err == nil {
			value := strings.TrimSpace(string(data))
			if value != "" {
				return value, nil
			}
		}
	}
	productName, _ := os.ReadFile(filepathJoin(opts.SysfsRoot, "class", "dmi", "id", "product_name"))
	text := strings.ToLower(strings.TrimSpace(string(productName)))
	if strings.Contains(text, "kvm") || strings.Contains(text, "qemu") {
		return "kvm", nil
	}
	if strings.Contains(text, "virtualbox") {
		return "virtualbox", nil
	}
	if strings.Contains(text, "vmware") {
		return "vmware", nil
	}
	return "none", nil
}

func contextBackground() context.Context { return context.Background() }
