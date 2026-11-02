package software

import (
	"context"
	"os/exec"
	"strings"
)

type CommandRunner interface {
	Run(context.Context, string, ...string) ([]byte, error)
}

type DetectOptions struct {
	Runner                    CommandRunner
	OSID                      string
	InitSystem                string
	LookPath                  func(string) (string, error)
	DefaultDisplayManagerPath string
}

type PackageManagerInfo struct {
	Name                         string
	Available                    bool
	Path                         string
	Detected                     bool
	Version                      string
	HostDistribution             string
	HostNative                   bool
	ConfiguredTargetDistribution string
	TargetPackageManager         string
	AdapterStatus                string
	Native                       bool
	MatchesTarget                bool
}

type CommandInfo struct {
	Name      string
	Installed bool
	Path      string
	Version   string
}

type ServiceInfo struct {
	Name      string
	State     string
	Available bool
	Installed bool
	Loaded    bool
	Scope     string
	Alias     string
	Role      string
}

type SoftwareReport struct {
	PackageManagers  []PackageManagerInfo
	Commands         []CommandInfo
	Services         []ServiceInfo
	DisplayManager   string
	SystemdAvailable bool
}

func Detect(opts DetectOptions) (SoftwareReport, []string, error) {
	warnings := []string{}
	packageManagers, warnings1 := detectPackageManagers(opts)
	warnings = append(warnings, warnings1...)
	commands, warnings2 := detectCommands(opts)
	warnings = append(warnings, warnings2...)
	services, systemdAvailable, warnings3 := detectServices(opts)
	warnings = append(warnings, warnings3...)
	displayManager := ""
	for _, service := range services {
		if service.Role == "display-manager" && service.State == "active" {
			displayManager = strings.TrimSuffix(service.Name, ".service")
			break
		}
	}
	return SoftwareReport{
		PackageManagers: packageManagers, Commands: commands, Services: services,
		DisplayManager: displayManager, SystemdAvailable: systemdAvailable,
	}, warnings, nil
}

func detectCommands(opts DetectOptions) ([]CommandInfo, []string) {
	warnings := []string{}
	commands := []struct {
		name string
		args []string
	}{
		{name: "git", args: []string{"--version"}},
		{name: "docker", args: []string{"--version"}},
		{name: "docker-compose", args: []string{"--version"}},
		{name: "go", args: []string{"version"}},
		{name: "python3", args: []string{"--version"}},
		{name: "uv", args: []string{"--version"}},
		{name: "node", args: []string{"--version"}},
		{name: "npm", args: []string{"--version"}},
		{name: "mise", args: []string{"--version"}},
		{name: "rustc", args: []string{"--version"}},
		{name: "cargo", args: []string{"--version"}},
		{name: "nvim", args: []string{"--version"}},
		{name: "code", args: []string{"--version"}},
		{name: "kitty", args: []string{"--version"}},
		{name: "alacritty", args: []string{"--version"}},
		{name: "hyprland", args: []string{"--version"}},
		{name: "waybar", args: []string{"--version"}},
		{name: "pipewire", args: []string{"--version"}},
		{name: "bluetoothctl", args: []string{"--version"}},
	}
	result := []CommandInfo{}
	for _, entry := range commands {
		path, err := exec.LookPath(entry.name)
		if err != nil {
			continue
		}
		version := ""
		if out, cmdErr := runCommand(opts, entry.name, entry.args...); cmdErr == nil {
			version = strings.TrimSpace(string(out))
			if version == "" {
				version = "available"
			}
		}
		result = append(result, CommandInfo{Name: entry.name, Installed: true, Path: path, Version: version})
	}
	return result, warnings
}

func runCommand(opts DetectOptions, name string, args ...string) ([]byte, error) {
	if opts.Runner != nil {
		return opts.Runner.Run(context.Background(), name, args...)
	}
	return exec.Command(name, args...).Output()
}

func lookPath(opts DetectOptions, name string) (string, error) {
	if opts.LookPath != nil {
		return opts.LookPath(name)
	}
	return exec.LookPath(name)
}
