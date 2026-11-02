package pacman

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"runtime"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"eed/internal/packageinfo"
)

const databaseStaleAfter = 7 * 24 * time.Hour

type Runner interface {
	Run(context.Context, string, ...string) ([]byte, error)
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	command := exec.CommandContext(ctx, name, args...)
	command.Env = append(os.Environ(), "LC_ALL=C", "LANG=C")
	return command.CombinedOutput()
}

type Inspector struct {
	Runner     Runner
	LookPath   func(string) (string, error)
	SyncDBPath string
	Now        func() time.Time
	StaleAfter time.Duration
}

func NewInspector() *Inspector {
	return &Inspector{
		Runner:     ExecRunner{},
		LookPath:   exec.LookPath,
		SyncDBPath: "/var/lib/pacman/sync",
		Now:        time.Now,
		StaleAfter: databaseStaleAfter,
	}
}

func (i *Inspector) Inspect(ctx context.Context, requested []string) (packageinfo.Report, error) {
	if ctx == nil {
		ctx = context.Background()
	}
	if i.LookPath != nil {
		if _, err := i.LookPath("pacman"); err != nil {
			return packageinfo.UnavailableReport(uniqueSorted(requested), "unavailable", "pacman is not installed"), nil
		}
	}
	runner := i.Runner
	if runner == nil {
		runner = ExecRunner{}
	}

	allOutput, err := runner.Run(ctx, "pacman", "-Q")
	if err != nil {
		return failedInventory(requested, "read installed package inventory", allOutput, err)
	}
	explicitOutput, err := runner.Run(ctx, "pacman", "-Qe")
	if err != nil {
		return failedInventory(requested, "read explicitly installed package inventory", explicitOutput, err)
	}
	installed, err := parseInstalledPackages(string(allOutput))
	if err != nil {
		return failedInventory(requested, "parse installed package inventory", allOutput, err)
	}
	explicit, err := parseInstalledPackages(string(explicitOutput))
	if err != nil {
		return failedInventory(requested, "parse explicitly installed package inventory", explicitOutput, err)
	}

	report := packageinfo.Report{
		Status:                   "available",
		MetadataStatus:           "available",
		ProviderResolutionStatus: "not_needed",
		InventoryKnown:           true,
		Candidates:               []packageinfo.Candidate{},
		ResolvedPackages:         []packageinfo.Package{},
		EstimateComplete:         true,
	}
	report.InstalledPackages = installedPackageList(installed)
	report.RepositoryDatabaseAge, report.RepositoryDatabaseStale = i.repositoryDatabaseAge()
	if report.RepositoryDatabaseAge == "" {
		report.MetadataStatus = "unavailable"
		report.EstimateComplete = false
		report.Warnings = append(report.Warnings, "cached repository database timestamp is unavailable; repository metadata may be missing or stale")
	} else if report.RepositoryDatabaseStale {
		report.MetadataStatus = "stale"
		report.Warnings = append(report.Warnings, "cached repository database is older than the freshness threshold; no database refresh was performed")
	}

	queued := uniqueSorted(requested)
	visited := map[string]bool{}
	metadata := map[string]packageinfo.Package{}
	candidateNames := uniqueSorted(requested)
	candidateSet := make(map[string]bool, len(candidateNames))
	for _, name := range candidateNames {
		candidateSet[name] = true
	}
	for len(queued) > 0 {
		if err := ctx.Err(); err != nil {
			return report, fmt.Errorf("inspect cached pacman metadata: %w", err)
		}
		name := normalizePackageName(queued[0])
		queued = queued[1:]
		if name == "" || visited[name] {
			continue
		}
		visited[name] = true

		output, queryErr := runner.Run(ctx, "pacman", "-Si", "--", name)
		if queryErr != nil {
			if isPackageUnavailable(queryErr, output) {
				if candidateSet[name] {
					report.Candidates = append(report.Candidates, candidate(name, installed, explicit, nil, packageinfo.StateRepositoryUnavailable, "package is not present in the cached enabled repositories"))
				} else if resolvedProvider(name, metadata) {
					report.ProviderResolutionStatus = "resolved_from_cached_package_metadata"
				} else {
					report.UnknownDependencies = append(report.UnknownDependencies, name)
					report.ProviderResolutionStatus = "partial"
					report.EstimateComplete = false
				}
				continue
			}
			report.MetadataStatus = "unavailable"
			report.EstimateComplete = false
			report.Warnings = append(report.Warnings, fmt.Sprintf("cached repository metadata for %s is unavailable: %s", name, commandFailure(output, queryErr)))
			if candidateSet[name] {
				report.Candidates = append(report.Candidates, candidate(name, installed, explicit, nil, packageinfo.StateUnknown, "cached repository metadata could not be queried"))
			} else {
				report.UnknownDependencies = append(report.UnknownDependencies, name)
				report.ProviderResolutionStatus = "partial"
			}
			continue
		}

		pkg, parseErr := parseSyncInfo(string(output))
		if parseErr != nil {
			report.MetadataStatus = "unavailable"
			report.EstimateComplete = false
			report.Errors = append(report.Errors, fmt.Sprintf("parse cached repository metadata for %s: %v", name, parseErr))
			if candidateSet[name] {
				report.Candidates = append(report.Candidates, candidate(name, installed, explicit, nil, packageinfo.StateUnknown, "cached package metadata is malformed"))
			} else {
				report.UnknownDependencies = append(report.UnknownDependencies, name)
				report.ProviderResolutionStatus = "partial"
			}
			continue
		}
		if !architectureMatches(pkg.Architecture) {
			if candidateSet[name] {
				report.Candidates = append(report.Candidates, candidate(name, installed, explicit, &pkg, packageinfo.StateRepositoryUnavailable, "package architecture is incompatible with this host"))
			} else {
				report.UnknownDependencies = append(report.UnknownDependencies, name)
				report.Warnings = append(report.Warnings, fmt.Sprintf("dependency %s is unavailable for architecture %s", name, pkg.Architecture))
			}
			report.ProviderResolutionStatus = "partial"
			report.EstimateComplete = false
			continue
		}
		metadata[name] = pkg
		if candidateSet[name] {
			report.Candidates = append(report.Candidates, candidate(name, installed, explicit, &pkg, candidateState(name, installed, pkg), candidateReason(name, installed, pkg)))
		}
		queued = append(queued, pkg.Dependencies...)
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
		if local, ok := installed[name]; ok {
			pkg.Installed = true
			pkg.InstalledVersion = local.version
			_, pkg.Explicit = explicit[name]
			pkg.ExplicitKnown = true
			pkg.State = candidateState(name, installed, pkg)
		} else {
			pkg.State = packageinfo.StateNotInstalled
		}
		report.ResolvedPackages = append(report.ResolvedPackages, pkg)
		report.TotalDownloadBytes += pkg.DownloadBytes
		report.TotalInstalledBytes += pkg.InstalledBytes
		if !pkg.DownloadSizeKnown || !pkg.InstalledSizeKnown {
			report.EstimateComplete = false
			report.Warnings = append(report.Warnings, fmt.Sprintf("cached size metadata is incomplete for %s", name))
		}
		if pkg.State == packageinfo.StateAlreadyInstalled {
			continue
		}
		report.AdditionalDownloadBytes += pkg.DownloadBytes
		report.AdditionalInstallBytes += pkg.InstalledBytes
	}
	if len(report.UnknownDependencies) > 0 {
		report.EstimateComplete = false
		report.Warnings = append(report.Warnings, fmt.Sprintf(
			"dependency metadata is incomplete for %d package names; a name may be missing or a virtual dependency whose provider could not be resolved from per-package cached metadata (%s)",
			len(report.UnknownDependencies), strings.Join(report.UnknownDependencies, ", ")))
	}
	if report.RepositoryDatabaseStale {
		report.EstimateComplete = false
	}
	report.TotalDownloadKnown = report.EstimateComplete
	report.TotalInstalledKnown = report.EstimateComplete
	report.AdditionalDownloadKnown = report.EstimateComplete
	report.AdditionalInstallKnown = report.EstimateComplete
	return report, nil
}

func (i *Inspector) Owns(ctx context.Context, path string) (bool, error) {
	runner := i.Runner
	if runner == nil {
		runner = ExecRunner{}
	}
	output, err := runner.Run(ctx, "pacman", "-Qo", "--", path)
	if err == nil {
		return true, nil
	}
	if strings.Contains(strings.ToLower(string(output)), "no package owns") {
		return false, nil
	}
	return false, fmt.Errorf("query pacman file ownership for %q: %s", path, commandFailure(output, err))
}

func (i *Inspector) Installed(ctx context.Context, name string) (packageinfo.Candidate, error) {
	report, err := i.Inspect(ctx, []string{name})
	if err != nil {
		return packageinfo.Candidate{}, err
	}
	for _, candidate := range report.Candidates {
		if candidate.Name == name {
			return candidate, nil
		}
	}
	return packageinfo.Candidate{Name: name, State: packageinfo.StateUnknown, RepositoryStatus: "unknown", Reason: "package inventory did not return a candidate"}, nil
}

type Approval struct{ state *approvalState }

type approvalState struct {
	transactionID string
	mu            sync.Mutex
	consumed      bool
}

type Transaction struct {
	Packages               []string `json:"packages"`
	FullSystemUpgrade      bool     `json:"full_system_upgrade"`
	DatabaseSync           bool     `json:"database_sync"`
	RequestsPackageRemoval bool     `json:"requests_package_removal"`
	Arguments              []string `json:"arguments"`
	ConfirmationNotice     string   `json:"confirmation_notice"`
	transactionID          string
}

var validPackageName = regexp.MustCompile(`^[A-Za-z0-9@._+-]+$`)

func BuildTransaction(packages []string) (Transaction, error) {
	names := uniqueSorted(packages)
	if len(names) == 0 {
		return Transaction{}, errors.New("package transaction has no targets")
	}
	for _, name := range names {
		if !validPackageName.MatchString(name) || strings.HasPrefix(name, "-") {
			return Transaction{}, fmt.Errorf("invalid pacman package target %q", name)
		}
	}
	transaction := Transaction{
		Packages:               names,
		FullSystemUpgrade:      true,
		DatabaseSync:           true,
		RequestsPackageRemoval: false,
		Arguments:              append([]string{"-Syu", "--needed", "--"}, names...),
		ConfirmationNotice:     "Pacman will synchronize repositories and may perform a full system upgrade. Review its complete transaction, including any conflict or removal prompts, and confirm it there.",
	}
	transaction.transactionID = transactionFingerprint(transaction)
	return transaction, nil
}

func ConfirmTransaction(transaction Transaction, input interface{ Read([]byte) (int, error) }, output interface{ Write([]byte) (int, error) }) (Approval, error) {
	if !validTransaction(transaction) {
		return Approval{}, errors.New("transaction is invalid or was modified after planning")
	}
	if _, err := fmt.Fprintf(output, "%s\nPackages: %s\nType YES to authorize EED to start this full-system-upgrade transaction: ",
		transaction.ConfirmationNotice, strings.Join(transaction.Packages, ", ")); err != nil {
		return Approval{}, fmt.Errorf("write transaction confirmation: %w", err)
	}
	var response string
	if _, err := fmt.Fscan(input, &response); err != nil {
		return Approval{}, fmt.Errorf("read transaction confirmation: %w", err)
	}
	if response != "YES" {
		return Approval{}, errors.New("package transaction was not confirmed")
	}
	return Approval{state: &approvalState{transactionID: transaction.transactionID}}, nil
}

func ExecuteTransaction(ctx context.Context, transaction Transaction, approval Approval, runner Runner) ([]byte, error) {
	return executeTransaction(ctx, transaction, approval, runner, os.Geteuid() == 0)
}

func executeTransaction(ctx context.Context, transaction Transaction, approval Approval, runner Runner, privileged bool) ([]byte, error) {
	if !privileged {
		return nil, errors.New("pacman transaction requires explicit privileged invocation; EED will not invoke sudo")
	}
	if !validTransaction(transaction) || approval.state == nil ||
		approval.state.transactionID != transaction.transactionID {
		return nil, errors.New("package transaction requires explicit confirmation for this exact transaction")
	}
	approval.state.mu.Lock()
	defer approval.state.mu.Unlock()
	if approval.state.consumed {
		return nil, errors.New("package transaction approval has already been used")
	}
	approval.state.consumed = true
	if runner == nil {
		runner = ExecRunner{}
	}
	output, err := runner.Run(ctx, "pacman", transaction.Arguments...)
	if err != nil {
		return output, fmt.Errorf("pacman full-system-upgrade transaction failed: %s", commandFailure(output, err))
	}
	return output, nil
}

func validTransaction(transaction Transaction) bool {
	if transaction.transactionID == "" ||
		transaction.transactionID != transactionFingerprint(transaction) ||
		len(transaction.Arguments) < 4 ||
		transaction.Arguments[0] != "-Syu" ||
		transaction.Arguments[1] != "--needed" ||
		transaction.Arguments[2] != "--" ||
		strings.Join(transaction.Packages, "\x00") != strings.Join(transaction.Arguments[3:], "\x00") ||
		!transaction.FullSystemUpgrade || !transaction.DatabaseSync || transaction.RequestsPackageRemoval {
		return false
	}
	for _, name := range transaction.Packages {
		if !validPackageName.MatchString(name) || strings.HasPrefix(name, "-") {
			return false
		}
	}
	return true
}

func transactionFingerprint(transaction Transaction) string {
	fields := []string{
		fmt.Sprintf("full-system-upgrade=%t", transaction.FullSystemUpgrade),
		fmt.Sprintf("database-sync=%t", transaction.DatabaseSync),
		fmt.Sprintf("requests-package-removal=%t", transaction.RequestsPackageRemoval),
		transaction.ConfirmationNotice,
	}
	fields = append(fields, transaction.Arguments...)
	return strings.Join(fields, "\x00")
}

func parseInstalledPackages(output string) (map[string]installedPackage, error) {
	packages := map[string]installedPackage{}
	for lineNumber, line := range strings.Split(output, "\n") {
		fields := strings.Fields(line)
		if len(fields) == 0 {
			continue
		}
		if len(fields) != 2 || !validPackageName.MatchString(fields[0]) {
			return nil, fmt.Errorf("invalid pacman package record on line %d", lineNumber+1)
		}
		packages[fields[0]] = installedPackage{version: fields[1]}
	}
	return packages, nil
}

func parseSyncInfo(output string) (packageinfo.Package, error) {
	pkg := packageinfo.Package{}
	fields := map[string]string{}
	var currentKey string
	for _, line := range strings.Split(output, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		if !strings.HasPrefix(line, " ") && strings.Contains(line, " : ") {
			key, value, _ := strings.Cut(line, " : ")
			currentKey = strings.TrimSpace(key)
			fields[currentKey] = strings.TrimSpace(value)
			continue
		}
		if currentKey != "" && strings.HasPrefix(line, " ") {
			fields[currentKey] += " " + strings.TrimSpace(line)
		}
	}
	pkg.Name = fields["Name"]
	pkg.Version = fields["Version"]
	pkg.Repository = fields["Repository"]
	pkg.Architecture = fields["Architecture"]
	if pkg.Name == "" || pkg.Version == "" || pkg.Architecture == "" {
		return packageinfo.Package{}, errors.New("required name, version, or architecture field is missing")
	}
	var err error
	if pkg.DownloadBytes, err = parseSize(fields["Download Size"]); err != nil {
		return packageinfo.Package{}, fmt.Errorf("download size: %w", err)
	}
	pkg.DownloadSizeKnown = sizeFieldKnown(fields["Download Size"])
	if pkg.InstalledBytes, err = parseSize(fields["Installed Size"]); err != nil {
		return packageinfo.Package{}, fmt.Errorf("installed size: %w", err)
	}
	pkg.InstalledSizeKnown = sizeFieldKnown(fields["Installed Size"])
	pkg.Dependencies = splitMetadataList(fields["Depends On"])
	pkg.OptionalDeps = splitMetadataList(fields["Optional Deps"])
	pkg.Provides = splitMetadataList(fields["Provides"])
	pkg.Conflicts = splitMetadataList(fields["Conflicts With"])
	pkg.Replaces = splitMetadataList(fields["Replaces"])
	return pkg, nil
}

func sizeFieldKnown(value string) bool {
	value = strings.TrimSpace(value)
	return value != "" && !strings.EqualFold(value, "None")
}

func parseSize(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if value == "" || value == "None" {
		return 0, nil
	}
	fields := strings.Fields(value)
	if len(fields) == 0 {
		return 0, errors.New("size is empty")
	}
	size, err := strconv.ParseFloat(fields[0], 64)
	if err != nil || size < 0 {
		return 0, fmt.Errorf("invalid size %q", value)
	}
	unit := "B"
	if len(fields) > 1 {
		unit = strings.TrimSuffix(fields[1], "B")
	}
	multipliers := map[string]float64{"B": 1, "Ki": 1 << 10, "Mi": 1 << 20, "Gi": 1 << 30, "Ti": 1 << 40}
	multiplier, ok := multipliers[unit]
	if !ok {
		return 0, fmt.Errorf("unsupported unit %q", unit)
	}
	return int64(size * multiplier), nil
}

func splitMetadataList(value string) []string {
	value = strings.TrimSpace(value)
	if value == "" || value == "None" {
		return nil
	}
	items := strings.Split(value, "  ")
	result := make([]string, 0, len(items))
	for _, item := range items {
		item = strings.TrimSpace(item)
		if item == "" {
			continue
		}
		if colon := strings.Index(item, ": "); colon >= 0 {
			item = item[:colon]
		}
		result = append(result, item)
	}
	return result
}

type installedPackage struct {
	version string
}

func installedPackageList(installed map[string]installedPackage) []packageinfo.InstalledPackage {
	names := make([]string, 0, len(installed))
	for name := range installed {
		names = append(names, name)
	}
	sort.Strings(names)
	packages := make([]packageinfo.InstalledPackage, 0, len(names))
	for _, name := range names {
		packages = append(packages, packageinfo.InstalledPackage{Name: name, Version: installed[name].version})
	}
	return packages
}

func candidate(name string, installed, explicit map[string]installedPackage, repo *packageinfo.Package, state, reason string) packageinfo.Candidate {
	result := packageinfo.Candidate{Name: name, State: state, RepositoryStatus: "available", Reason: reason}
	if repo == nil {
		result.RepositoryStatus = "unavailable"
	} else {
		result.RepositoryVersion = repo.Version
	}
	if local, ok := installed[name]; ok {
		result.InstalledVersion = local.version
	}
	if _, ok := installed[name]; ok {
		value := false
		if _, ok := explicit[name]; ok {
			value = true
		}
		result.Explicit = &value
	}
	return result
}

func candidateState(name string, installed map[string]installedPackage, repo packageinfo.Package) string {
	local, ok := installed[name]
	if !ok {
		return packageinfo.StateNotInstalled
	}
	if local.version != repo.Version {
		return packageinfo.StateVersionMismatch
	}
	return packageinfo.StateAlreadyInstalled
}

func candidateReason(name string, installed map[string]installedPackage, repo packageinfo.Package) string {
	state := candidateState(name, installed, repo)
	if state == packageinfo.StateVersionMismatch {
		return "installed and cached repository versions differ"
	}
	if state == packageinfo.StateAlreadyInstalled {
		return "installed package matches cached repository version"
	}
	return ""
}

func isPackageUnavailable(err error, output []byte) bool {
	lower := strings.ToLower(string(output) + " " + err.Error())
	return strings.Contains(lower, "target not found") || strings.Contains(lower, "package not found")
}

func commandFailure(output []byte, err error) string {
	detail := strings.TrimSpace(string(output))
	if detail == "" {
		return err.Error()
	}
	return detail
}

func failedInventory(requested []string, operation string, output []byte, err error) (packageinfo.Report, error) {
	report := packageinfo.UnavailableReport(uniqueSorted(requested), "unavailable", "installed package inventory is unavailable")
	report.Errors = append(report.Errors, fmt.Sprintf("%s: %s", operation, commandFailure(output, err)))
	for index := range report.Candidates {
		report.Candidates[index].Reason = "Pacman local package database could not be read"
	}
	return report, nil
}

func (i *Inspector) repositoryDatabaseAge() (string, bool) {
	path := i.SyncDBPath
	if path == "" {
		path = "/var/lib/pacman/sync"
	}
	entries, err := os.ReadDir(path)
	if err != nil {
		return "", false
	}
	var latest time.Time
	for _, entry := range entries {
		if entry.IsDir() || !strings.HasSuffix(entry.Name(), ".db") {
			continue
		}
		info, err := entry.Info()
		if err == nil && info.ModTime().After(latest) {
			latest = info.ModTime()
		}
	}
	if latest.IsZero() {
		return "", false
	}
	now := time.Now()
	if i.Now != nil {
		now = i.Now()
	}
	staleAfter := i.StaleAfter
	if staleAfter <= 0 {
		staleAfter = databaseStaleAfter
	}
	age := now.Sub(latest)
	if age < 0 {
		age = 0
	}
	return age.Round(time.Hour).String(), age > staleAfter
}

func resolvedProvider(name string, packages map[string]packageinfo.Package) bool {
	for _, pkg := range packages {
		for _, provided := range pkg.Provides {
			if normalizePackageName(provided) == name {
				return true
			}
		}
	}
	return false
}

func architectureMatches(architecture string) bool {
	return architecture == "any" || architecture == runtimeArchitecture()
}

func runtimeArchitecture() string {
	switch runtime.GOARCH {
	case "amd64":
		return "x86_64"
	case "arm64":
		return "aarch64"
	default:
		return runtime.GOARCH
	}
}

func normalizePackageName(value string) string {
	value = strings.TrimSpace(value)
	if index := strings.IndexAny(value, "<>="); index >= 0 {
		value = value[:index]
	}
	return strings.TrimSpace(value)
}

func uniqueSorted(values []string) []string {
	seen := map[string]bool{}
	result := make([]string, 0, len(values))
	for _, value := range values {
		value = normalizePackageName(value)
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		result = append(result, value)
	}
	sort.Strings(result)
	return result
}

func sortCandidates(values []packageinfo.Candidate) []packageinfo.Candidate {
	sort.Slice(values, func(i, j int) bool { return values[i].Name < values[j].Name })
	return values
}

func SyncDatabasePath(configPath string) string {
	if configPath != "" {
		return filepath.Clean(configPath)
	}
	return "/var/lib/pacman/sync"
}
