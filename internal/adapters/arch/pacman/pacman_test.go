package pacman

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"eed/internal/packageinfo"
)

type fakeRunner struct {
	responses map[string][]byte
	errors    map[string]error
	calls     [][]string
}

func (r *fakeRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	call := append([]string{name}, args...)
	r.calls = append(r.calls, call)
	key := strings.Join(call, " ")
	return r.responses[key], r.errors[key]
}

func TestInspectUsesOnlyCachedPacmanQueriesAndComparesInventory(t *testing.T) {
	syncDB := t.TempDir()
	db := filepath.Join(syncDB, "extra.db")
	if err := os.WriteFile(db, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err := os.Chtimes(db, now.Add(-time.Hour), now.Add(-time.Hour)); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{
		responses: map[string][]byte{
			"pacman -Q":             []byte("git 1.0-1\ncurl 8.0-1\n"),
			"pacman -Qe":            []byte("git 1.0-1\n"),
			"pacman -Si -- git":     []byte("Repository      : extra\nName            : git\nVersion         : 2.0-1\nDescription     : Version control\nArchitecture    : x86_64\nURL             : https://example.invalid\nLicenses        : GPL\nGroups          : None\nProvides        : None\nDepends On      : curl  openssl\nOptional Deps   : less: pager\nConflicts With  : None\nReplaces        : None\nDownload Size   : 1.00 MiB\nInstalled Size  : 4.00 MiB\n"),
			"pacman -Si -- curl":    []byte("Repository      : core\nName            : curl\nVersion         : 8.0-1\nDescription     : transfer utility\nArchitecture    : x86_64\nDepends On      : openssl\nOptional Deps   : None\nProvides        : None\nConflicts With  : None\nReplaces        : None\nDownload Size   : 512.00 KiB\nInstalled Size  : 2.00 MiB\n"),
			"pacman -Si -- openssl": []byte("Repository      : core\nName            : openssl\nVersion         : 3.0-1\nDescription     : crypto library\nArchitecture    : x86_64\nDepends On      : None\nOptional Deps   : None\nProvides        : libcrypto.so\nConflicts With  : None\nReplaces        : None\nDownload Size   : 256.00 KiB\nInstalled Size  : 1.00 MiB\n"),
		},
	}
	inspector := &Inspector{
		Runner: runner,
		LookPath: func(name string) (string, error) {
			if name != "pacman" {
				t.Fatalf("unexpected executable lookup: %s", name)
			}
			return "/usr/bin/pacman", nil
		},
		SyncDBPath: syncDB,
		Now:        func() time.Time { return now },
		StaleAfter: 7 * 24 * time.Hour,
	}
	report, err := inspector.Inspect(context.Background(), []string{"git"})
	if err != nil {
		t.Fatal(err)
	}
	if report.MetadataStatus != "available" || report.RepositoryDatabaseStale {
		t.Fatalf("expected fresh cached repository metadata: %+v", report)
	}
	if len(report.Candidates) != 1 || report.Candidates[0].State != "version_mismatch" ||
		report.Candidates[0].InstalledVersion != "1.0-1" || report.Candidates[0].RepositoryVersion != "2.0-1" ||
		report.Candidates[0].Explicit == nil || !*report.Candidates[0].Explicit {
		t.Fatalf("expected installed explicit version comparison: %+v", report.Candidates)
	}
	if !reflect.DeepEqual(report.InstalledPackages, []packageinfo.InstalledPackage{
		{Name: "curl", Version: "8.0-1"}, {Name: "git", Version: "1.0-1"},
	}) {
		t.Fatalf("installed package inventory should expose sorted names and versions: %+v", report.InstalledPackages)
	}
	if len(report.ResolvedPackages) != 3 || report.TotalInstalledBytes != (4+2+1)*(1<<20) ||
		report.AdditionalInstallBytes != (4+1)*(1<<20) ||
		report.AdditionalDownloadBytes != (1<<20)+(256)*(1<<10) {
		t.Fatalf("expected additional sizes to exclude already installed runtime dependencies: %+v", report)
	}
	resolved := map[string]packageinfo.Package{}
	for _, pkg := range report.ResolvedPackages {
		resolved[pkg.Name] = pkg
	}
	if resolved["git"].State != packageinfo.StateVersionMismatch ||
		!resolved["git"].Installed || !resolved["git"].Explicit ||
		resolved["curl"].State != packageinfo.StateAlreadyInstalled || resolved["curl"].Explicit ||
		resolved["openssl"].State != packageinfo.StateNotInstalled {
		t.Fatalf("resolved dependencies should expose installed/explicit state: %+v", report.ResolvedPackages)
	}
	wantCalls := [][]string{
		{"pacman", "-Q"},
		{"pacman", "-Qe"},
		{"pacman", "-Si", "--", "git"},
		{"pacman", "-Si", "--", "curl"},
		{"pacman", "-Si", "--", "openssl"},
	}
	if !reflect.DeepEqual(runner.calls, wantCalls) {
		t.Fatalf("unexpected commands; diagnostics must not sync or modify package DB: %#v", runner.calls)
	}
}

func TestInspectDistinguishesUnavailableAndUnknownMetadata(t *testing.T) {
	runner := &fakeRunner{
		responses: map[string][]byte{
			"pacman -Q":  []byte(""),
			"pacman -Qe": []byte(""),
		},
		errors: map[string]error{
			"pacman -Si -- missing-repo-package": errors.New("exit status 1"),
			"pacman -Si -- dependency":           errors.New("exit status 1"),
		},
	}
	runner.responses["pacman -Si -- missing-repo-package"] = []byte("error: target not found: missing-repo-package")
	runner.responses["pacman -Si -- existing-package"] = []byte("Repository : extra\nName : existing-package\nVersion : 1-1\nArchitecture : x86_64\nDepends On : dependency\nDownload Size : None\nInstalled Size : 1 MiB\n")
	inspector := &Inspector{
		Runner: runner, LookPath: func(string) (string, error) { return "pacman", nil },
		SyncDBPath: t.TempDir(), Now: time.Now,
	}
	report, err := inspector.Inspect(context.Background(), []string{"missing-repo-package", "existing-package"})
	if err != nil {
		t.Fatal(err)
	}
	states := map[string]string{}
	for _, candidate := range report.Candidates {
		states[candidate.Name] = candidate.State
	}
	if states["missing-repo-package"] != "repository_unavailable" ||
		states["existing-package"] != "not_installed" {
		t.Fatalf("expected unavailable and not-installed states: %+v", report.Candidates)
	}
	if len(report.UnknownDependencies) != 1 || report.UnknownDependencies[0] != "dependency" ||
		report.EstimateComplete || report.ProviderResolutionStatus != "partial" ||
		!strings.Contains(strings.Join(report.Warnings, " "), "virtual dependency") {
		t.Fatalf("unknown dependency metadata must remain explicit and incomplete: %+v", report)
	}
}

func TestInspectResolvesVirtualDependencyFromCachedPackageProvides(t *testing.T) {
	runner := &fakeRunner{
		responses: map[string][]byte{
			"pacman -Q":                  []byte(""),
			"pacman -Qe":                 []byte(""),
			"pacman -Si -- consumer":     []byte("Repository : extra\nName : consumer\nVersion : 1-1\nArchitecture : x86_64\nDepends On : virtual-api\nDownload Size : 1 MiB\nInstalled Size : 2 MiB\n"),
			"pacman -Si -- provider-pkg": []byte("Repository : extra\nName : provider-pkg\nVersion : 1-1\nArchitecture : x86_64\nDepends On : None\nProvides : virtual-api=1\nDownload Size : 1 MiB\nInstalled Size : 2 MiB\n"),
		},
		errors: map[string]error{"pacman -Si -- virtual-api": errors.New("exit status 1")},
	}
	runner.responses["pacman -Si -- virtual-api"] = []byte("error: target not found: virtual-api")
	report, err := (&Inspector{
		Runner: runner, LookPath: func(string) (string, error) { return "pacman", nil },
		SyncDBPath: t.TempDir(), Now: time.Now,
	}).Inspect(context.Background(), []string{"consumer", "provider-pkg"})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.UnknownDependencies) != 0 || report.ProviderResolutionStatus != "resolved_from_cached_package_metadata" {
		t.Fatalf("known cached Provides metadata should satisfy a virtual dependency: %+v", report)
	}
}

func TestInspectReportsStaleDatabaseAndInaccessibleInventory(t *testing.T) {
	syncDB := t.TempDir()
	db := filepath.Join(syncDB, "core.db")
	if err := os.WriteFile(db, []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	if err := os.Chtimes(db, now.Add(-30*24*time.Hour), now.Add(-30*24*time.Hour)); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{
		responses: map[string][]byte{"pacman -Q": []byte("error: failed to initialize alpm library\n")},
		errors:    map[string]error{"pacman -Q": errors.New("exit status 1")},
	}
	inspector := &Inspector{
		Runner: runner, LookPath: func(string) (string, error) { return "pacman", nil },
		SyncDBPath: syncDB, Now: func() time.Time { return now }, StaleAfter: time.Hour,
	}
	report, err := inspector.Inspect(context.Background(), []string{"git"})
	if err != nil {
		t.Fatal(err)
	}
	if report.Status != "unavailable" || len(report.Errors) != 1 ||
		!strings.Contains(report.Errors[0], "failed to initialize alpm") ||
		len(report.Candidates) != 1 || report.Candidates[0].State != "unknown" {
		t.Fatalf("inaccessible local database must be clearly reported: %+v", report)
	}
}

func TestOwnsUsesReadOnlyPacmanQueryAndDistinguishesUnownedFiles(t *testing.T) {
	runner := &fakeRunner{
		responses: map[string][]byte{
			"pacman -Qo -- /usr/bin/git":   []byte("/usr/bin/git is owned by git 2.0-1\n"),
			"pacman -Qo -- /tmp/user-file": []byte("error: No package owns /tmp/user-file\n"),
		},
		errors: map[string]error{"pacman -Qo -- /tmp/user-file": errors.New("exit status 1")},
	}
	inspector := &Inspector{Runner: runner}
	owned, err := inspector.Owns(context.Background(), "/usr/bin/git")
	if err != nil || !owned {
		t.Fatalf("expected Pacman-owned file: owned=%v err=%v", owned, err)
	}
	owned, err = inspector.Owns(context.Background(), "/tmp/user-file")
	if err != nil || owned {
		t.Fatalf("expected unowned file without error: owned=%v err=%v", owned, err)
	}
	wantCalls := [][]string{
		{"pacman", "-Qo", "--", "/usr/bin/git"},
		{"pacman", "-Qo", "--", "/tmp/user-file"},
	}
	if !reflect.DeepEqual(runner.calls, wantCalls) {
		t.Fatalf("unexpected file ownership queries: %#v", runner.calls)
	}
}

func TestBuildTransactionOnlyGeneratesFullUpgradeWithNativeConfirmation(t *testing.T) {
	transaction, err := BuildTransaction([]string{"waybar", "git", "git"})
	if err != nil {
		t.Fatal(err)
	}
	want := []string{"-Syu", "--needed", "--", "git", "waybar"}
	if !reflect.DeepEqual(transaction.Arguments, want) ||
		!transaction.FullSystemUpgrade || !transaction.DatabaseSync || transaction.RequestsPackageRemoval {
		t.Fatalf("unsafe pacman transaction generated: %+v", transaction)
	}
	for _, arg := range transaction.Arguments {
		if strings.Contains(arg, "--noconfirm") || arg == "-Sy" || arg == "-S" {
			t.Fatalf("transaction contains unsafe partial or auto-confirm option: %#v", transaction.Arguments)
		}
	}
	tamperedNotice := transaction
	tamperedNotice.ConfirmationNotice = "Install packages only"
	if _, err := ConfirmTransaction(tamperedNotice, strings.NewReader("YES\n"), &strings.Builder{}); err == nil {
		t.Fatal("confirmation notice must not be mutable after transaction planning")
	}
	var output strings.Builder
	if _, err := ConfirmTransaction(transaction, strings.NewReader("NO\n"), &output); err == nil {
		t.Fatal("transaction should require explicit user confirmation")
	}
	approval, err := ConfirmTransaction(transaction, strings.NewReader("YES\n"), &output)
	if err != nil || approval.state == nil || approval.state.transactionID == "" || !strings.Contains(output.String(), "full system upgrade") {
		t.Fatalf("expected explicit approval and upgrade notice: approval=%+v output=%q err=%v", approval, output.String(), err)
	}
	transaction.Arguments = []string{"-Sy", "git"}
	if _, err := ExecuteTransaction(context.Background(), transaction, approval, &fakeRunner{}); err == nil {
		t.Fatal("modified or partial-upgrade transaction must not execute")
	}
}

func TestTransactionExecutionRequiresAndConsumesExactApproval(t *testing.T) {
	transaction, err := BuildTransaction([]string{"git"})
	if err != nil {
		t.Fatal(err)
	}
	var output strings.Builder
	approval, err := ConfirmTransaction(transaction, strings.NewReader("YES\n"), &output)
	if err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{}
	if _, err := executeTransaction(context.Background(), transaction, approval, runner, false); err == nil {
		t.Fatal("non-privileged transaction execution must be rejected")
	}
	if len(runner.calls) != 0 {
		t.Fatalf("rejected transaction must not reach runner: %#v", runner.calls)
	}
	if _, err := executeTransaction(context.Background(), transaction, approval, runner, true); err != nil {
		t.Fatalf("approved transaction should execute through injected runner: %v", err)
	}
	if len(runner.calls) != 1 || strings.Join(runner.calls[0], " ") != "pacman -Syu --needed -- git" {
		t.Fatalf("unexpected transaction runner calls: %#v", runner.calls)
	}
	if _, err := executeTransaction(context.Background(), transaction, approval, runner, true); err == nil {
		t.Fatal("a confirmed transaction approval must be one-use")
	}
	if len(runner.calls) != 1 {
		t.Fatalf("reused approval must not invoke pacman again: %#v", runner.calls)
	}
}

func TestBuildTransactionRejectsArgumentInjection(t *testing.T) {
	for _, name := range []string{"-S", "git;echo bad", "git name"} {
		if _, err := BuildTransaction([]string{name}); err == nil {
			t.Errorf("expected package target %q to be rejected", name)
		}
	}
}

func TestSyncInfoParsesRepositoryMetadataWithoutBuildDependencies(t *testing.T) {
	pkg, err := parseSyncInfo("Repository : extra\nName : app\nVersion : 1-2\nArchitecture : x86_64\nDepends On : lib>=1  virtual-api\nOptional Deps : optional: feature\nProvides : virtual-api=1\nConflicts With : old-app\nReplaces : old-app\nDownload Size : 1.5 MiB\nInstalled Size : 4 KiB\n")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(pkg.Dependencies, []string{"lib>=1", "virtual-api"}) ||
		!reflect.DeepEqual(pkg.OptionalDeps, []string{"optional"}) ||
		!reflect.DeepEqual(pkg.Provides, []string{"virtual-api=1"}) ||
		!reflect.DeepEqual(pkg.Conflicts, []string{"old-app"}) ||
		!reflect.DeepEqual(pkg.Replaces, []string{"old-app"}) {
		t.Fatalf("unexpected parsed runtime metadata: %+v", pkg)
	}
	if pkg.DownloadBytes != int64(1.5*1024*1024) || pkg.InstalledBytes != 4*1024 {
		t.Fatalf("unexpected package sizes: %+v", pkg)
	}
	if !pkg.DownloadSizeKnown || !pkg.InstalledSizeKnown {
		t.Fatalf("numeric repository sizes should be explicitly marked known: %+v", pkg)
	}
	pkg, err = parseSyncInfo("Repository : extra\nName : app\nVersion : 1-2\nArchitecture : x86_64\nDownload Size : None\nInstalled Size : 4 KiB\n")
	if err != nil {
		t.Fatal(err)
	}
	if pkg.DownloadSizeKnown || !pkg.InstalledSizeKnown {
		t.Fatalf("unknown sizes must remain distinguishable from measured zero: %+v", pkg)
	}
}
