package apt

import (
	"context"
	"errors"
	"reflect"
	"strings"
	"testing"

	"eed/internal/packageinfo"
)

type commandResult struct {
	output string
	err    error
}

type mockRunner struct {
	results map[string]commandResult
	calls   [][]string
}

func (m *mockRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	call := append([]string{name}, args...)
	m.calls = append(m.calls, call)
	key := strings.Join(call, "\x00")
	result, ok := m.results[key]
	if !ok {
		return nil, errors.New("unexpected command: " + strings.Join(call, " "))
	}
	return []byte(result.output), result.err
}

func commandKey(values ...string) string {
	return strings.Join(values, "\x00")
}

func TestInspectParsesDpkgAndCachedAPTMetadataReadOnly(t *testing.T) {
	runner := &mockRunner{results: map[string]commandResult{
		commandKey("dpkg-query", "-W", "-f=${binary:Package}\\t${Version}\\t${db:Status-Status}\\n"): {
			output: "tool:amd64\t1.0\tinstalled\n",
		},
		commandKey("apt-cache", "policy", "tool"): {
			output: "tool:\n  Installed: 1.0\n  Candidate: 1.0\n",
		},
		commandKey("apt-cache", "show", "tool=1.0"): {
			output: "Package: tool\nVersion: 1.0\nArchitecture: amd64\nSize: 0\nInstalled-Size: 0\n",
		},
	}}
	inspector := &Inspector{
		Runner: runner,
		LookPath: func(name string) (string, error) {
			if name != "dpkg-query" && name != "apt-cache" {
				t.Fatalf("unexpected executable lookup: %s", name)
			}
			return "/usr/bin/" + name, nil
		},
	}

	report, err := inspector.Inspect(context.Background(), []string{"tool"})
	if err != nil {
		t.Fatal(err)
	}
	if !report.InventoryKnown || report.Status != "available" || len(report.Candidates) != 1 {
		t.Fatalf("expected known package inventory: %+v", report)
	}
	if !reflect.DeepEqual(report.InstalledPackages, []packageinfo.InstalledPackage{{Name: "tool:amd64", Version: "1.0"}}) {
		t.Fatalf("installed package inventory should include names and versions: %+v", report.InstalledPackages)
	}
	candidate := report.Candidates[0]
	if candidate.State != packageinfo.StateAlreadyInstalled || candidate.InstalledVersion != "1.0" {
		t.Fatalf("unexpected installed candidate state: %+v", candidate)
	}
	if !report.EstimateComplete || !report.TotalDownloadKnown || !report.TotalInstalledKnown ||
		!report.AdditionalDownloadKnown || !report.AdditionalInstallKnown {
		t.Fatalf("zero sizes are known values, not missing estimates: %+v", report)
	}
	if report.TotalDownloadBytes != 0 || report.TotalInstalledBytes != 0 ||
		report.AdditionalDownloadBytes != 0 || report.AdditionalInstallBytes != 0 {
		t.Fatalf("expected explicit zero sizes: %+v", report)
	}
	for _, call := range runner.calls {
		if call[0] != "dpkg-query" && call[0] != "apt-cache" {
			t.Fatalf("unexpected command executed: %v", call)
		}
		if strings.Contains(strings.Join(call, " "), "update") ||
			strings.Contains(strings.Join(call, " "), "install") ||
			strings.Contains(strings.Join(call, " "), "upgrade") {
			t.Fatalf("mutating APT command was executed: %v", call)
		}
	}
}

func TestInspectDistinguishesUnavailableCandidateAndUnknownSize(t *testing.T) {
	runner := &mockRunner{results: map[string]commandResult{
		commandKey("dpkg-query", "-W", "-f=${binary:Package}\\t${Version}\\t${db:Status-Status}\\n"): {
			output: "tool\t1.0\tinstalled\n",
		},
		commandKey("apt-cache", "policy", "tool"): {
			output: "tool:\n  Installed: 1.0\n  Candidate: (none)\n",
		},
	}}
	inspector := &Inspector{
		Runner:   runner,
		LookPath: func(string) (string, error) { return "/usr/bin/tool", nil },
	}
	report, err := inspector.Inspect(context.Background(), []string{"tool"})
	if err != nil {
		t.Fatal(err)
	}
	if !report.InventoryKnown || report.Candidates[0].State != packageinfo.StateAlreadyInstalled ||
		report.Candidates[0].RepositoryStatus != "unavailable" {
		t.Fatalf("installed state should not be lost when repository candidate is unavailable: %+v", report)
	}
	if report.TotalInstalledKnown || report.AdditionalInstallKnown || report.EstimateComplete {
		t.Fatalf("unavailable metadata must not become a zero-sized estimate: %+v", report)
	}
}

func TestInspectDoesNotTreatIncompleteDpkgStateAsNotInstalled(t *testing.T) {
	runner := &mockRunner{results: map[string]commandResult{
		commandKey("dpkg-query", "-W", "-f=${binary:Package}\\t${Version}\\t${db:Status-Status}\\n"): {
			output: "tool\t1.0\tunpacked\n",
		},
		commandKey("apt-cache", "policy", "tool"): {
			output: "tool:\n  Candidate: 1.0\n",
		},
		commandKey("apt-cache", "show", "tool=1.0"): {
			output: "Package: tool\nVersion: 1.0\nSize: 10\nInstalled-Size: 20\n",
		},
	}}
	report, err := (&Inspector{
		Runner:   runner,
		LookPath: func(string) (string, error) { return "/usr/bin/tool", nil },
	}).Inspect(context.Background(), []string{"tool"})
	if err != nil {
		t.Fatal(err)
	}
	if report.Candidates[0].State != packageinfo.StateUnknown ||
		report.ResolvedPackages[0].State != packageinfo.StateUnknown ||
		report.EstimateComplete || report.AdditionalInstallKnown || report.AdditionalInstallBytes != 0 {
		t.Fatalf("incomplete dpkg state must not be estimated as a fresh install: %+v", report)
	}
	if len(report.InstalledPackages) != 0 {
		t.Fatalf("unpacked packages must not be listed as fully installed: %+v", report.InstalledPackages)
	}
}

func TestParsePackageRecordPreservesZeroAndUnknownSizes(t *testing.T) {
	zero, err := parsePackageRecord("Package: zero\nVersion: 1\nSize: 0\nInstalled-Size: 0\n")
	if err != nil {
		t.Fatal(err)
	}
	if !zero.DownloadSizeKnown || !zero.InstalledSizeKnown || zero.DownloadBytes != 0 || zero.InstalledBytes != 0 {
		t.Fatalf("explicit zero sizes must be known: %+v", zero)
	}

	unknown, err := parsePackageRecord("Package: unknown\nVersion: 1\n")
	if err != nil {
		t.Fatal(err)
	}
	if unknown.DownloadSizeKnown || unknown.InstalledSizeKnown {
		t.Fatalf("missing size metadata must remain unknown: %+v", unknown)
	}
}

func TestParsePackageRecordHandlesContinuationAndParagraphBoundaries(t *testing.T) {
	record := "Package: tool\nVersion: 1.0\nDepends: libc6 (>= 2.3),\n zlib1g | zlib1g-dev\n\n" +
		"Package: unrelated\nVersion: 9.9\n"
	pkg, err := parsePackageRecord(record)
	if err != nil {
		t.Fatal(err)
	}
	if pkg.Name != "tool" || pkg.Version != "1.0" ||
		!reflect.DeepEqual(pkg.Dependencies, []string{"libc6", "zlib1g"}) {
		t.Fatalf("APT continuation or paragraph parsing failed: %+v", pkg)
	}
	if !hasDependencyAlternatives(record) {
		t.Fatal("alternative dependencies must be identified as unresolved")
	}
}

func TestParseInstalledFiltersNonInstalledRecords(t *testing.T) {
	installed, err := parseInstalled("tool:amd64\t1.2\tinstalled\nold\t1.1\tconfig-files\n")
	if err != nil {
		t.Fatal(err)
	}
	if len(installed) != 2 || installed["tool"].version != "1.2" ||
		installed["tool"].status != "installed" || installed["old"].status != "config-files" {
		t.Fatalf("unexpected dpkg inventory: %+v", installed)
	}
}

func TestParseCandidateVersionAndDependencyNames(t *testing.T) {
	if version, available, known := parseCandidateVersion("Candidate: 1.2\n"); version != "1.2" || !available || !known {
		t.Fatalf("unexpected candidate parse: %q %t %t", version, available, known)
	}
	if version, available, known := parseCandidateVersion("Candidate: (none)\n"); version != "" || available || !known {
		t.Fatalf("unexpected unavailable candidate parse: %q %t %t", version, available, known)
	}
	if names := parseDependencyNames("libc6 (>= 2.3), foo:any"); !reflect.DeepEqual(names, []string{"foo", "libc6"}) {
		t.Fatalf("unexpected dependency parse: %#v", names)
	}
}
