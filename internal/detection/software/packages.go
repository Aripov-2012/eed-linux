package software

import (
	"os/exec"
	"strings"
)

const installationTargetManager = "pacman"
const installationTargetDistribution = "arch"

func detectPackageManagers(opts DetectOptions) ([]PackageManagerInfo, []string) {
	nativeManager := nativePackageManager(opts.OSID)
	hostDistribution := strings.ToLower(strings.TrimSpace(opts.OSID))
	if hostDistribution == "" {
		hostDistribution = "unknown"
	}
	candidates := []string{"apt", "pacman", "dnf", "zypper", "apk"}
	lookPath := opts.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}
	result := make([]PackageManagerInfo, 0, len(candidates))
	for _, name := range candidates {
		path, err := lookPath(name)
		available := err == nil
		hostNative := name == nativeManager
		matchesTarget := name == installationTargetManager
		if !available && !hostNative && !matchesTarget {
			continue
		}
		info := PackageManagerInfo{
			Name: name, Available: available, Path: path, Detected: available,
			HostDistribution: hostDistribution, HostNative: hostNative,
			ConfiguredTargetDistribution: installationTargetDistribution,
			TargetPackageManager:         installationTargetManager,
			AdapterStatus:                packageManagerAdapterStatus(name),
			Native:                       hostNative, MatchesTarget: matchesTarget,
		}
		if available {
			if version, runErr := runCommand(opts, name, packageManagerVersionArgs(name)...); runErr == nil {
				info.Version = strings.TrimSpace(string(version))
			}
		}
		result = append(result, info)
	}
	return result, nil
}

func packageManagerAdapterStatus(name string) string {
	switch name {
	case "apt", "pacman":
		return "implemented"
	default:
		return "not_implemented"
	}
}

func nativePackageManager(osID string) string {
	switch strings.ToLower(strings.TrimSpace(osID)) {
	case "arch", "manjaro", "endeavouros", "artix", "garuda":
		return "pacman"
	case "ubuntu", "debian", "linuxmint", "pop", "raspbian", "elementary":
		return "apt"
	case "fedora":
		return "dnf"
	case "opensuse", "opensuse-leap", "opensuse-tumbleweed", "sles", "sled":
		return "zypper"
	case "alpine":
		return "apk"
	default:
		return ""
	}
}

func packageManagerVersionArgs(name string) []string {
	switch name {
	case "pacman":
		return []string{"--version"}
	case "apt":
		return []string{"--version"}
	case "dnf":
		return []string{"--version"}
	case "zypper":
		return []string{"--version"}
	case "apk":
		return []string{"--version"}
	default:
		return []string{"--version"}
	}
}
