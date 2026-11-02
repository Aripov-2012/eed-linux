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

func TestSyncInfoSeparatesRuntimeOptionalAndBuildDependencies(t *testing.T) {
	pkg, err := parseSyncInfo("Repository : extra\nName : app\nVersion : 1-2\nArchitecture : x86_64\nDepends On : lib>=1  virtual-api\nOptional Deps : optional: feature\nMake Depends On : build-tool>=2\nCheck Depends On : test-tool\nProvides : virtual-api=1\nConflicts With : old-app\nReplaces : old-app\nDownload Size : 1.5 MiB\nInstalled Size : 4 KiB\n")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(pkg.Dependencies, []string{"lib>=1", "virtual-api"}) ||
		!reflect.DeepEqual(pkg.OptionalDeps, []string{"optional"}) ||
		!reflect.DeepEqual(pkg.BuildDependencies, []string{"build-tool>=2"}) ||
		!reflect.DeepEqual(pkg.CheckDependencies, []string{"test-tool"}) ||
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

func TestDependencyConstraintAndSonameProviderMatching(t *testing.T) {
	runner := &fakeRunner{responses: map[string][]byte{
		"vercmp 3 3": []byte("0"),
		"vercmp 3 4": []byte("-1"),
		"vercmp 4 4": []byte("0"),
	}}
	provider := packageinfo.Package{Name: "openssl", Version: "3.1-1", Provides: []string{"libssl.so=3"}}
	matches, known := packageSatisfies(provider, "libssl.so>=3", runner, context.Background())
	if !matches || !known {
		t.Fatalf("versioned soname should match its declared provider metadata: matches=%t known=%t", matches, known)
	}
	matches, known = packageSatisfies(provider, "libssl.so=4", runner, context.Background())
	if matches || !known {
		t.Fatalf("mismatched versioned soname should not be accepted: matches=%t known=%t", matches, known)
	}
	curlProvider := packageinfo.Package{Name: "curl", Version: "8.0-1", Provides: []string{"libcurl.so=4"}}
	matches, known = packageSatisfies(curlProvider, "libcurl.so>=4", runner, context.Background())
	if !matches || !known {
		t.Fatalf("libcurl.so should resolve from its declared versioned provider: matches=%t known=%t", matches, known)
	}
}

func TestInspectUsesFileDatabaseForSonameDependencies(t *testing.T) {
	runner := &fakeRunner{
		responses: map[string][]byte{
			"pacman -Q":               []byte(""),
			"pacman -Qe":              []byte(""),
			"pacman -Si -- consumer":  []byte("Repository : extra\nName : consumer\nVersion : 1-1\nArchitecture : x86_64\nDepends On : libssl.so=3-64\nDownload Size : 1 MiB\nInstalled Size : 2 MiB\n"),
			"pacman -Fq -- libssl.so": []byte("core/openssl\n"),
			"pacman -Si -- openssl":   []byte("Repository : core\nName : openssl\nVersion : 3.1-1\nArchitecture : x86_64\nProvides : libssl.so=3-64\nDownload Size : 1 MiB\nInstalled Size : 2 MiB\n"),
			"vercmp 3-64 3-64":        []byte("0"),
			"pacman -Si -- libssl.so": []byte("error: target not found: libssl.so"),
		},
		errors: map[string]error{
			"pacman -Si -- libssl.so": errors.New("exit status 1"),
		},
	}
	report, err := (&Inspector{
		Runner: runner, LookPath: func(string) (string, error) { return "pacman", nil },
		SyncDBPath: t.TempDir(), Now: time.Now,
	}).Inspect(context.Background(), []string{"consumer"})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.UnknownDependencies) != 0 || len(report.ResolvedPackages) != 2 {
		t.Fatalf("file database owner should resolve the soname dependency: %+v", report)
	}
}

func TestInspectResolvesVirtualPackageProviderFromSyncMetadata(t *testing.T) {
	runner := &fakeRunner{
		responses: map[string][]byte{
			"pacman -Q":                   []byte(""),
			"pacman -Qe":                  []byte(""),
			"pacman -Si -- app":           []byte("Repository : extra\nName : app\nVersion : 1-1\nArchitecture : x86_64\nDepends On : opengl-driver\nDownload Size : 1 MiB\nInstalled Size : 2 MiB\n"),
			"pacman -Slq":                 []byte("mesa\n"),
			"pacman -Si -- mesa":          []byte("Repository : extra\nName : mesa\nVersion : 1-1\nArchitecture : x86_64\nProvides : opengl-driver\nDownload Size : 1 MiB\nInstalled Size : 2 MiB\n"),
			"pacman -Si -- opengl-driver": []byte("error: target not found: opengl-driver"),
		},
		errors: map[string]error{"pacman -Si -- opengl-driver": errors.New("exit status 1")},
	}
	report, err := (&Inspector{
		Runner: runner, LookPath: func(string) (string, error) { return "pacman", nil },
		SyncDBPath: t.TempDir(), Now: time.Now,
	}).Inspect(context.Background(), []string{"app"})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.UnknownDependencies) != 0 || !containsPackage(report.ResolvedPackages, "mesa") {
		t.Fatalf("opengl-driver should resolve from declared Provides metadata: %+v", report)
	}
}

func TestInspectKeepsMissingProviderUnresolved(t *testing.T) {
	syncDB := t.TempDir()
	if err := os.WriteFile(filepath.Join(syncDB, "core.db"), []byte("fixture"), 0o600); err != nil {
		t.Fatal(err)
	}
	runner := &fakeRunner{
		responses: map[string][]byte{
			"pacman -Q":                       []byte(""),
			"pacman -Qe":                      []byte(""),
			"pacman -Si -- app":               []byte("Repository : extra\nName : app\nVersion : 1-1\nArchitecture : x86_64\nDepends On : missing-virtual\nDownload Size : 1 MiB\nInstalled Size : 2 MiB\n"),
			"pacman -Slq":                     []byte("unrelated-package\n"),
			"pacman -Si -- unrelated-package": []byte("Repository : core\nName : unrelated-package\nVersion : 1-1\nArchitecture : x86_64\nProvides : other-virtual\nDownload Size : 1 MiB\nInstalled Size : 2 MiB\n"),
			"pacman -Si -- missing-virtual":   []byte("error: target not found: missing-virtual"),
		},
		errors: map[string]error{"pacman -Si -- missing-virtual": errors.New("exit status 1")},
	}
	report, err := (&Inspector{
		Runner: runner, LookPath: func(string) (string, error) { return "pacman", nil },
		SyncDBPath: syncDB, Now: time.Now,
	}).Inspect(context.Background(), []string{"app"})
	if err != nil {
		t.Fatal(err)
	}
	if report.MetadataStatus != "available" || len(report.UnknownDependencies) != 1 ||
		report.UnknownDependencies[0] != "missing-virtual" || report.EstimateComplete {
		t.Fatalf("missing providers must remain unresolved rather than be invented: %+v", report)
	}
}

func TestInspectAppliesOrdinaryDependencyVersionConstraints(t *testing.T) {
	runner := &fakeRunner{
		responses: map[string][]byte{
			"pacman -Q":          []byte(""),
			"pacman -Qe":         []byte(""),
			"pacman -Si -- app":  []byte("Repository : extra\nName : app\nVersion : 1-1\nArchitecture : x86_64\nDepends On : lib>=2\nDownload Size : 1 MiB\nInstalled Size : 2 MiB\n"),
			"pacman -Si -- lib":  []byte("Repository : core\nName : lib\nVersion : 1-1\nArchitecture : x86_64\nDownload Size : 1 MiB\nInstalled Size : 2 MiB\n"),
			"vercmp 1-1 2":       []byte("-1"),
			"pacman -Slq":        []byte("lib2\n"),
			"pacman -Si -- lib2": []byte("Repository : core\nName : lib2\nVersion : 2-1\nArchitecture : x86_64\nProvides : lib=2\nDownload Size : 1 MiB\nInstalled Size : 2 MiB\n"),
			"vercmp 2 2":         []byte("0"),
		},
	}
	report, err := (&Inspector{
		Runner: runner, LookPath: func(string) (string, error) { return "pacman", nil },
		SyncDBPath: t.TempDir(), Now: time.Now,
	}).Inspect(context.Background(), []string{"app"})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.UnknownDependencies) != 0 || !containsPackage(report.ResolvedPackages, "lib2") ||
		containsPackage(report.ResolvedPackages, "lib") {
		t.Fatalf("dependency version constraints should choose a declared matching provider: %+v", report)
	}
}

func containsPackage(packages []packageinfo.Package, name string) bool {
	for _, pkg := range packages {
		if pkg.Name == name {
			return true
		}
	}
	return false
}

func TestInspectUsesFileDatabaseForLibcurlSonameDependency(t *testing.T) {
	runner := &fakeRunner{
		responses: map[string][]byte{
			"pacman -Q":                []byte(""),
			"pacman -Qe":               []byte(""),
			"pacman -Si -- http-tool":  []byte("Repository : extra\nName : http-tool\nVersion : 1.0-1\nArchitecture : x86_64\nDepends On : libcurl.so>=4-64\nDownload Size : 1 MiB\nInstalled Size : 2 MiB\n"),
			"pacman -Fq -- libcurl.so": []byte("core/curl\n"),
			"pacman -Si -- curl":       []byte("Repository : core\nName : curl\nVersion : 8.5.0-1\nArchitecture : x86_64\nProvides : libcurl.so=4-64\nDownload Size : 1.2 MiB\nInstalled Size : 3 MiB\n"),
			"vercmp 4-64 4-64":         []byte("0"),
			"pacman -Si -- libcurl.so": []byte("error: target not found: libcurl.so"),
		},
		errors: map[string]error{
			"pacman -Si -- libcurl.so": errors.New("exit status 1"),
		},
	}
	report, err := (&Inspector{
		Runner: runner, LookPath: func(string) (string, error) { return "pacman", nil },
		SyncDBPath: t.TempDir(), Now: time.Now,
	}).Inspect(context.Background(), []string{"http-tool"})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.UnknownDependencies) != 0 || len(report.ResolvedPackages) != 2 ||
		!containsPackage(report.ResolvedPackages, "curl") || !containsPackage(report.ResolvedPackages, "http-tool") {
		t.Fatalf("file database owner should resolve the libcurl soname dependency: %+v", report)
	}
}

func TestInspectPreservesOptionalDependenciesWithoutResolvingThem(t *testing.T) {
	runner := &fakeRunner{
		responses: map[string][]byte{
			"pacman -Q":              []byte(""),
			"pacman -Qe":             []byte(""),
			"pacman -Si -- app":      []byte("Repository : extra\nName : app\nVersion : 1-1\nArchitecture : x86_64\nDepends On : base-dep\nOptional Deps : opt-feature: nice feature\nMake Depends On : build-tool\nDownload Size : 1 MiB\nInstalled Size : 2 MiB\n"),
			"pacman -Si -- base-dep": []byte("Repository : core\nName : base-dep\nVersion : 1-1\nArchitecture : x86_64\nDownload Size : 500 KiB\nInstalled Size : 1 MiB\n"),
		},
	}
	report, err := (&Inspector{
		Runner: runner, LookPath: func(string) (string, error) { return "pacman", nil },
		SyncDBPath: t.TempDir(), Now: time.Now,
	}).Inspect(context.Background(), []string{"app"})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.ResolvedPackages) != 2 || !containsPackage(report.ResolvedPackages, "base-dep") ||
		containsPackage(report.ResolvedPackages, "opt-feature") || containsPackage(report.ResolvedPackages, "build-tool") {
		t.Fatalf("optional and build dependencies must not be resolved as runtime packages: %+v", report)
	}
	for _, pkg := range report.ResolvedPackages {
		if pkg.Name == "app" {
			if !reflect.DeepEqual(pkg.OptionalDeps, []string{"opt-feature"}) {
				t.Fatalf("optional dependencies should be preserved in package metadata: %+v", pkg)
			}
			if !reflect.DeepEqual(pkg.BuildDependencies, []string{"build-tool"}) {
				t.Fatalf("build dependencies should be preserved in package metadata: %+v", pkg)
			}
		}
	}
}

func TestDependencyClassification(t *testing.T) {
	cases := []struct {
		dep      string
		isSoname bool
		isVirt   bool
	}{
		{"libssl.so=3-64", true, false},
		{"libcurl.so>=4-64", true, false},
		{"libnghttp2.so", true, false},
		{"opengl-driver", false, true},
		{"cron", false, true},
		{"git>=2.0", false, false},
		{"cairo", false, false},
	}
	for _, tc := range cases {
		c := ClassifyDependency(tc.dep)
		if c.IsSONAME != tc.isSoname {
			t.Errorf("dep %q: expected IsSONAME=%t, got %t", tc.dep, tc.isSoname, c.IsSONAME)
		}
		if c.IsVirtual != tc.isVirt {
			t.Errorf("dep %q: expected IsVirtual=%t, got %t", tc.dep, tc.isVirt, c.IsVirtual)
		}
	}
}

func TestInspectNeverQueriesSONAMEOrVirtualAsOrdinaryPackages(t *testing.T) {
	runner := &fakeRunner{
		responses: map[string][]byte{
			"pacman -Q":                   []byte(""),
			"pacman -Qe":                  []byte(""),
			"pacman -Si -- graphics-app":  []byte("Repository : extra\nName : graphics-app\nVersion : 1-1\nArchitecture : x86_64\nDepends On : libnghttp2.so  opengl-driver\nDownload Size : 1 MiB\nInstalled Size : 2 MiB\n"),
			"pacman -Fq -- libnghttp2.so": []byte("core/libnghttp2\n"),
			"pacman -Si -- libnghttp2":    []byte("Repository : core\nName : libnghttp2\nVersion : 1.60.0-1\nArchitecture : x86_64\nProvides : None\nDownload Size : 1 MiB\nInstalled Size : 2 MiB\n"),
			"pacman -Slq":                 []byte("mesa\n"),
			"pacman -Si -- mesa":          []byte("Repository : extra\nName : mesa\nVersion : 24.0-1\nArchitecture : x86_64\nProvides : opengl-driver\nDownload Size : 10 MiB\nInstalled Size : 30 MiB\n"),
		},
	}
	inspector := &Inspector{
		Runner: runner, LookPath: func(string) (string, error) { return "pacman", nil },
		SyncDBPath: t.TempDir(), Now: time.Now,
	}
	report, err := inspector.Inspect(context.Background(), []string{"graphics-app"})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.UnknownDependencies) != 0 {
		t.Fatalf("expected all dependencies resolved, got unknown: %v", report.UnknownDependencies)
	}
	// Verify that pacman -Si was NEVER called for libnghttp2.so or opengl-driver
	for _, call := range runner.calls {
		cmdStr := strings.Join(call, " ")
		if strings.Contains(cmdStr, "-Si -- libnghttp2.so") {
			t.Fatalf("forbidden call: queried SONAME as ordinary package: %s", cmdStr)
		}
		if strings.Contains(cmdStr, "-Si -- opengl-driver") {
			t.Fatalf("forbidden call: queried virtual dependency as ordinary package: %s", cmdStr)
		}
	}
}

func TestTransactionEngineRejectsNonArchHost(t *testing.T) {
	engine := NewTransactionEngine(&fakeRunner{}, nil, "ubuntu")
	transaction, err := BuildTransaction([]string{"git"})
	if err != nil {
		t.Fatal(err)
	}
	approval := Approval{state: &approvalState{transactionID: transaction.transactionID}}
	record, err := engine.Execute(context.Background(), transaction, approval, []string{"developer.git"})
	if err == nil {
		t.Fatal("expected error rejecting Ubuntu host")
	}
	if !strings.Contains(err.Error(), "unsupported host operating system \"ubuntu\"") {
		t.Fatalf("unexpected error message: %v", err)
	}
	if record.Success {
		t.Fatal("record should not report success")
	}
}

func TestTransactionEngineElevatedExecutionAndOwnershipTracking(t *testing.T) {
	runner := &fakeRunner{
		responses: map[string][]byte{
			"pacman -Q":         []byte("curl 8.0-1\n"),
			"pacman -Qe":        []byte(""),
			"pacman -Si -- git": []byte("Repository : extra\nName : git\nVersion : 2.0-1\nArchitecture : x86_64\nDownload Size : 1 MiB\nInstalled Size : 2 MiB\n"),
			"sudo pacman -Syu --needed -- git waybar": []byte("Packages (2) git-2.0-1 waybar-1.0-1\nTotal Installed Size: 5.00 MiB\n"),
		},
	}
	inspector := &Inspector{
		Runner: runner, LookPath: func(string) (string, error) { return "pacman", nil },
		SyncDBPath: t.TempDir(), Now: time.Now,
	}

	engine := NewTransactionEngine(runner, inspector, "arch")
	engine.UseSudo = true // test unprivileged invocation with sudo

	transaction, err := BuildTransaction([]string{"git", "waybar"})
	if err != nil {
		t.Fatal(err)
	}
	approval := Approval{state: &approvalState{transactionID: transaction.transactionID}}

	// After running the transaction, pacman -Q will report git and waybar installed
	record, err := engine.Execute(context.Background(), transaction, approval, []string{"developer.git", "desktop.hyprland"})
	if err != nil {
		t.Fatal(err)
	}
	if !record.Success {
		t.Fatalf("expected transaction success, got errors: %v", record.Errors)
	}
	// Check that sudo was invoked
	sudoCallFound := false
	for _, call := range runner.calls {
		if strings.Join(call, " ") == "sudo pacman -Syu --needed -- git waybar" {
			sudoCallFound = true
			break
		}
	}
	if !sudoCallFound {
		t.Fatalf("expected sudo pacman call, calls were: %#v", runner.calls)
	}
}
