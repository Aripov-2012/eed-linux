package apt

import (
	"context"
	"errors"
	"fmt"
	"os/exec"
	"sort"
	"strconv"
	"strings"

	"eed/internal/packageinfo"
)

type Runner interface {
	Run(context.Context, string, ...string) ([]byte, error)
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, name, args...)
	return command.CombinedOutput()
}

type Inspector struct {
	Runner   Runner
	LookPath func(string) (string, error)
}

func NewInspector() *Inspector {
	return &Inspector{Runner: ExecRunner{}, LookPath: exec.LookPath}
}

func (i *Inspector) Inspect(ctx context.Context, requested []string) (packageinfo.Report, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if i.LookPath != nil {
		if _, err := i.LookPath("dpkg-query"); err != nil {
			return packageinfo.UnavailableReport(uniqueSorted(requested), "unavailable", "dpkg-query is not installed"), nil
		}
		if _, err := i.LookPath("apt-cache"); err != nil {
			return packageinfo.UnavailableReport(uniqueSorted(requested), "unavailable", "apt-cache is not installed"), nil
		}
	}
	runner := i.Runner
	if runner == nil {
		runner = ExecRunner{}
	}
	output, err := runner.Run(ctx, "dpkg-query", "-W", "-f=${binary:Package}\\t${Version}\\t${db:Status-Status}\\n")
	if err != nil {
		report := packageinfo.UnavailableReport(uniqueSorted(requested), "unavailable", "dpkg package inventory could not be read")
		report.Errors = append(report.Errors, fmt.Sprintf("query installed dpkg packages: %s", commandFailure(output, err)))
		return report, nil
	}
	installed, err := parseInstalled(string(output))
	if err != nil {
		report := packageinfo.UnavailableReport(uniqueSorted(requested), "unavailable", "dpkg package inventory is malformed")
		report.Errors = append(report.Errors, err.Error())
		return report, nil
	}

	report := packageinfo.Report{
		Status: "available", MetadataStatus: "cached", ProviderResolutionStatus: "not_needed",
		InventoryKnown: true,
		Candidates:     []packageinfo.Candidate{}, ResolvedPackages: []packageinfo.Package{},
		EstimateComplete: true,
	}
	report.InstalledPackages = parseInstalledPackageList(string(output))
	metadata := make(map[string]packageinfo.Package)
	candidateSet := make(map[string]bool)
	for _, name := range uniqueSorted(requested) {
		candidateSet[name] = true
	}
	queue := uniqueSorted(requested)
	visited := make(map[string]bool)
	for len(queue) > 0 {
		if err := ctx.Err(); err != nil {
			return report, fmt.Errorf("inspect cached APT metadata: %w", err)
		}
		name := normalizeName(queue[0])
		queue = queue[1:]
		if name == "" || visited[name] {
			continue
		}
		visited[name] = true

		policy, policyErr := runner.Run(ctx, "apt-cache", "policy", name)
		if policyErr != nil {
			report.MetadataStatus = "incomplete"
			report.EstimateComplete = false
			report.Warnings = append(report.Warnings, fmt.Sprintf("cached APT policy for %s is unavailable: %s", name, commandFailure(policy, policyErr)))
			if candidateSet[name] {
				report.Candidates = append(report.Candidates, makeCandidate(name, installed, "", packageinfo.StateUnknown, "cached APT policy could not be read"))
			} else {
				report.ProviderResolutionStatus = "partial"
				report.UnknownDependencies = append(report.UnknownDependencies, name)
			}
			continue
		}
		version, hasCandidate, policyKnown := parseCandidateVersion(string(policy))
		if !policyKnown {
			report.MetadataStatus = "incomplete"
			report.EstimateComplete = false
			report.Warnings = append(report.Warnings, fmt.Sprintf("cached APT policy for %s has no recognizable candidate metadata", name))
			if candidateSet[name] {
				report.Candidates = append(report.Candidates, makeCandidate(name, installed, "", packageinfo.StateUnknown, "candidate repository metadata is unknown"))
			} else {
				report.ProviderResolutionStatus = "partial"
				report.UnknownDependencies = append(report.UnknownDependencies, name)
			}
			continue
		}
		if !hasCandidate {
			if candidateSet[name] {
				state, reason := packageinfo.StateRepositoryUnavailable, "no candidate version exists in cached APT metadata"
				if local, ok := installed[name]; ok && local.status == "installed" {
					state, reason = packageinfo.StateAlreadyInstalled, "package is installed but no candidate version exists in cached APT metadata"
				} else if ok && installationStateUnknown(local.status) {
					state, reason = packageinfo.StateUnknown, "dpkg reports an incomplete package installation state"
				}
				candidate := makeCandidate(name, installed, "", state, reason)
				if state == packageinfo.StateAlreadyInstalled {
					candidate.RepositoryStatus = "unavailable"
				}
				report.Candidates = append(report.Candidates, candidate)
				report.EstimateComplete = false
			} else {
				report.ProviderResolutionStatus = "partial"
				report.UnknownDependencies = append(report.UnknownDependencies, name)
				report.EstimateComplete = false
			}
			continue
		}

		infoOutput, infoErr := runner.Run(ctx, "apt-cache", "show", name+"="+version)
		if infoErr != nil {
			report.MetadataStatus = "incomplete"
			report.EstimateComplete = false
			report.Warnings = append(report.Warnings, fmt.Sprintf("cached APT package metadata for %s=%s is unavailable: %s", name, version, commandFailure(infoOutput, infoErr)))
			if candidateSet[name] {
				report.Candidates = append(report.Candidates, makeCandidate(name, installed, version, packageinfo.StateUnknown, "candidate package metadata could not be read"))
			} else {
				report.ProviderResolutionStatus = "partial"
				report.UnknownDependencies = append(report.UnknownDependencies, name)
			}
			continue
		}
		pkg, parseErr := parsePackageRecord(string(infoOutput))
		if parseErr != nil {
			report.MetadataStatus = "incomplete"
			report.EstimateComplete = false
			report.Errors = append(report.Errors, fmt.Sprintf("parse cached APT metadata for %s: %v", name, parseErr))
			if candidateSet[name] {
				report.Candidates = append(report.Candidates, makeCandidate(name, installed, version, packageinfo.StateUnknown, "candidate package metadata is malformed"))
			} else {
				report.ProviderResolutionStatus = "partial"
				report.UnknownDependencies = append(report.UnknownDependencies, name)
			}
			continue
		}
		if pkg.Name != name || pkg.Version != version {
			report.MetadataStatus = "incomplete"
			report.EstimateComplete = false
			report.Errors = append(report.Errors, fmt.Sprintf("APT returned metadata for %s=%s while querying %s=%s", pkg.Name, pkg.Version, name, version))
			continue
		}
		metadata[name] = pkg
		if hasDependencyAlternatives(string(infoOutput)) {
			report.ProviderResolutionStatus = "partial"
			report.EstimateComplete = false
			report.Warnings = append(report.Warnings,
				fmt.Sprintf("alternative dependency choices for %s were not resolved by the read-only inventory", name))
		}
		if candidateSet[name] {
			local, exists := installed[name]
			state := packageinfo.StateNotInstalled
			reason := ""
			if exists && local.status == "installed" && local.version == version {
				state, reason = packageinfo.StateAlreadyInstalled, "installed version matches the cached APT candidate"
			} else if exists && local.status == "installed" {
				state, reason = packageinfo.StateVersionMismatch, "installed and cached APT candidate versions differ"
			} else if exists && installationStateUnknown(local.status) {
				state, reason = packageinfo.StateUnknown, "dpkg reports an incomplete package installation state"
			}
			report.Candidates = append(report.Candidates, makeCandidate(name, installed, version, state, reason))
		}
		queue = append(queue, pkg.Dependencies...)
	}
	report.Candidates = sortCandidates(report.Candidates)
	report.UnknownDependencies = uniqueSorted(report.UnknownDependencies)
	names := make([]string, 0, len(metadata))
	for name := range metadata {
		names = append(names, name)
	}
	sort.Strings(names)
	for _, name := range names {
		pkg := metadata[name]
		if local, ok := installed[name]; ok && local.status == "installed" {
			pkg.Installed = true
			pkg.InstalledVersion = local.version
			if local.version == pkg.Version {
				pkg.State = packageinfo.StateAlreadyInstalled
			} else {
				pkg.State = packageinfo.StateVersionMismatch
			}
		} else if ok && installationStateUnknown(local.status) {
			pkg.State = packageinfo.StateUnknown
			report.EstimateComplete = false
			report.Warnings = append(report.Warnings,
				fmt.Sprintf("dpkg installation state is incomplete for %s (%s)", name, local.status))
		} else {
			pkg.State = packageinfo.StateNotInstalled
		}
		report.ResolvedPackages = append(report.ResolvedPackages, pkg)
		report.TotalDownloadBytes += pkg.DownloadBytes
		report.TotalInstalledBytes += pkg.InstalledBytes
		if !pkg.DownloadSizeKnown || !pkg.InstalledSizeKnown {
			report.EstimateComplete = false
			report.Warnings = append(report.Warnings, fmt.Sprintf("cached APT size metadata is incomplete for %s", name))
		}
		if pkg.State == packageinfo.StateAlreadyInstalled {
			continue
		}
		if pkg.State == packageinfo.StateUnknown {
			continue
		}
		report.AdditionalDownloadBytes += pkg.DownloadBytes
		report.AdditionalInstallBytes += pkg.InstalledBytes
	}
	if len(report.UnknownDependencies) > 0 {
		report.EstimateComplete = false
		report.Warnings = append(report.Warnings, fmt.Sprintf(
			"dependency metadata is incomplete for %d package names (%s)",
			len(report.UnknownDependencies), strings.Join(report.UnknownDependencies, ", ")))
	}
	report.TotalDownloadKnown = report.EstimateComplete
	report.TotalInstalledKnown = report.EstimateComplete
	report.AdditionalDownloadKnown = report.EstimateComplete
	report.AdditionalInstallKnown = report.EstimateComplete
	return report, nil
}

type installedPackage struct {
	version string
	status  string
}

func parseInstalled(output string) (map[string]installedPackage, error) {
	packages := make(map[string]installedPackage)
	for lineNo, line := range strings.Split(output, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		fields := strings.Split(line, "\t")
		if len(fields) != 3 || strings.TrimSpace(fields[0]) == "" || strings.TrimSpace(fields[1]) == "" ||
			strings.TrimSpace(fields[2]) == "" {
			return nil, fmt.Errorf("invalid dpkg-query record on line %d", lineNo+1)
		}
		name := normalizeName(fields[0])
		packages[name] = installedPackage{version: strings.TrimSpace(fields[1]), status: strings.TrimSpace(fields[2])}
	}
	return packages, nil
}

func installationStateUnknown(status string) bool {
	return status != "" && status != "installed" && status != "config-files" && status != "not-installed"
}

func parseCandidateVersion(output string) (string, bool, bool) {
	for _, line := range strings.Split(output, "\n") {
		key, value, ok := strings.Cut(strings.TrimSpace(line), ":")
		if !ok || key != "Candidate" {
			continue
		}
		value = strings.TrimSpace(value)
		if value == "(none)" || value == "" {
			return "", false, true
		}
		return value, true, true
	}
	return "", false, false
}

func parsePackageRecord(output string) (packageinfo.Package, error) {
	fields := make(map[string]string)
	currentKey := ""
	for _, line := range strings.Split(output, "\n") {
		if strings.TrimSpace(line) == "" {
			break
		}
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			if isPackageListField(currentKey) {
				fields[currentKey] += " " + strings.TrimSpace(line)
			}
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		currentKey = strings.TrimSpace(key)
		fields[currentKey] = strings.TrimSpace(value)
	}
	pkg := packageinfo.Package{
		Name: fields["Package"], Version: fields["Version"], Architecture: fields["Architecture"],
		Dependencies: parseDependencyNames(fields["Pre-Depends"], fields["Depends"], fields["Recommends"]),
		OptionalDeps: parseDependencyNames(fields["Suggests"]),
		Provides:     splitList(fields["Provides"]), Conflicts: parseDependencyNames(fields["Conflicts"], fields["Breaks"]),
		Replaces: splitList(fields["Replaces"]),
	}
	if pkg.Name == "" || pkg.Version == "" {
		return packageinfo.Package{}, errors.New("required Package or Version field is missing")
	}
	if value := strings.TrimSpace(fields["Size"]); value != "" {
		size, err := strconv.ParseInt(value, 10, 64)
		if err != nil || size < 0 {
			return packageinfo.Package{}, fmt.Errorf("invalid Size field %q", value)
		}
		pkg.DownloadBytes, pkg.DownloadSizeKnown = size, true
	}
	if value := strings.TrimSpace(fields["Installed-Size"]); value != "" {
		size, err := strconv.ParseInt(value, 10, 64)
		if err != nil || size < 0 || size > int64(^uint64(0)>>1)/1024 {
			return packageinfo.Package{}, fmt.Errorf("invalid Installed-Size field %q", value)
		}
		pkg.InstalledBytes, pkg.InstalledSizeKnown = size*1024, true
	}
	return pkg, nil
}

func isPackageListField(name string) bool {
	switch name {
	case "Pre-Depends", "Depends", "Recommends", "Suggests", "Provides", "Conflicts", "Breaks", "Replaces":
		return true
	default:
		return false
	}
}

func hasDependencyAlternatives(output string) bool {
	currentKey := ""
	for _, line := range strings.Split(output, "\n") {
		if strings.TrimSpace(line) == "" {
			break
		}
		if strings.HasPrefix(line, " ") || strings.HasPrefix(line, "\t") {
			if (currentKey == "Pre-Depends" || currentKey == "Depends" || currentKey == "Recommends") &&
				strings.Contains(line, "|") {
				return true
			}
			continue
		}
		key, value, ok := strings.Cut(line, ":")
		if !ok {
			continue
		}
		currentKey = strings.TrimSpace(key)
		if (currentKey == "Pre-Depends" || currentKey == "Depends" || currentKey == "Recommends") &&
			strings.Contains(value, "|") {
			return true
		}
	}
	return false
}

func parseDependencyNames(values ...string) []string {
	var names []string
	for _, value := range values {
		for _, dependency := range strings.Split(value, ",") {
			fields := strings.Fields(strings.TrimSpace(dependency))
			if len(fields) == 0 {
				continue
			}
			name := fields[0]
			name = strings.TrimSuffix(name, ":any")
			name = strings.TrimSuffix(name, ":native")
			names = append(names, name)
		}
	}
	return uniqueSorted(names)
}

func splitList(value string) []string {
	return parseDependencyNames(value)
}

func makeCandidate(name string, installed map[string]installedPackage, version, state, reason string) packageinfo.Candidate {
	candidate := packageinfo.Candidate{
		Name: name, State: state, RepositoryStatus: "available",
		RepositoryVersion: version, Reason: reason,
	}
	if state == packageinfo.StateRepositoryUnavailable {
		candidate.RepositoryStatus = "unavailable"
	}
	if local, ok := installed[name]; ok && local.status != "config-files" && local.status != "not-installed" {
		candidate.InstalledVersion = local.version
	}
	return candidate
}

func normalizeName(value string) string {
	value = strings.TrimSpace(value)
	if index := strings.IndexByte(value, ':'); index >= 0 {
		value = value[:index]
	}
	return value
}

func uniqueSorted(values []string) []string {
	seen := make(map[string]bool)
	var result []string
	for _, value := range values {
		value = normalizeName(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func parseInstalledPackageList(output string) []packageinfo.InstalledPackage {
	packages := make([]packageinfo.InstalledPackage, 0)
	for _, line := range strings.Split(output, "\n") {
		fields := strings.Split(line, "\t")
		if len(fields) != 3 || strings.TrimSpace(fields[2]) != "installed" {
			continue
		}
		packages = append(packages, packageinfo.InstalledPackage{
			Name: strings.TrimSpace(fields[0]), Version: strings.TrimSpace(fields[1]),
		})
	}
	sort.Slice(packages, func(i, j int) bool {
		if packages[i].Name == packages[j].Name {
			return packages[i].Version < packages[j].Version
		}
		return packages[i].Name < packages[j].Name
	})
	return packages
}

func sortCandidates(values []packageinfo.Candidate) []packageinfo.Candidate {
	sort.Slice(values, func(i, j int) bool { return values[i].Name < values[j].Name })
	return values
}

func commandFailure(output []byte, err error) string {
	detail := strings.TrimSpace(string(output))
	if detail == "" {
		return err.Error()
	}
	return detail
}
