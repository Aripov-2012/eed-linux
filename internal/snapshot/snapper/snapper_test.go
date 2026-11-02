package snapper

import (
	"context"
	"errors"
	"strings"
	"testing"

	"eed/internal/snapshot"
)

func TestParseSnapshotList(t *testing.T) {
	data := `Type | # | Pre # | Date | User | Cleanup | Description | Userdata
single | 0 | | 2025-01-01T00:00:00Z | root | | before-upgrade | eed=true`
	entries, err := ParseSnapshotList(data)
	if err != nil {
		t.Fatalf("ParseSnapshotList returned error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 snapshot, got %d", len(entries))
	}
	if entries[0].Number != 0 {
		t.Fatalf("expected snapshot number 0, got %d", entries[0].Number)
	}
	if !entries[0].EEDOwned {
		t.Fatal("expected snapshot to be marked EED-owned")
	}
}

func TestBuildCreateArgs(t *testing.T) {
	args := BuildCreateArgs(snapshot.CreateRequest{ConfigName: "root", Description: "Before EED install", ReadOnly: true})
	if len(args) == 0 {
		t.Fatal("expected create args")
	}
	if len(args) < 3 || args[0] != "-c" || args[1] != "root" || args[2] != "create" {
		t.Fatalf("expected configuration before create subcommand, got %#v", args)
	}
	foundReadOnly := false
	for _, arg := range args {
		if arg == "--read-only" {
			foundReadOnly = true
		}
	}
	if !foundReadOnly {
		t.Fatal("create args must always request a read-only snapshot")
	}
}

func TestDescriptionDoesNotEstablishEEDOwnership(t *testing.T) {
	entries, err := ParseSnapshotList("single | 7 | | 2025-01-01T00:00:00Z | root | | EED backup | other=value")
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].EEDOwned {
		t.Fatalf("description alone must not establish ownership: %+v", entries)
	}
}

type backendRunner struct {
	responses [][]byte
	errors    []error
	calls     [][]string
}

func (r *backendRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	call := append([]string{name}, args...)
	r.calls = append(r.calls, call)
	index := len(r.calls) - 1
	if index < len(r.errors) && r.errors[index] != nil {
		return nil, r.errors[index]
	}
	if index >= len(r.responses) {
		return nil, nil
	}
	return r.responses[index], nil
}

func TestCreateRequiresElevationAndReadOnly(t *testing.T) {
	backend := &Backend{
		Runner:     &backendRunner{},
		LookPath:   func(string) (string, error) { return "/usr/bin/snapper", nil },
		IsElevated: func() bool { return false },
	}
	if _, err := backend.Create(context.Background(), snapshot.CreateRequest{ConfigName: "root", ReadOnly: true}); err == nil {
		t.Fatal("expected privilege requirement")
	}
	backend.IsElevated = func() bool { return true }
	if _, err := backend.Create(context.Background(), snapshot.CreateRequest{ConfigName: "root", ReadOnly: false}); err == nil {
		t.Fatal("expected writable snapshot request to be rejected")
	}
}

func TestProbeReportsMissingSnapperWithoutRunningCommands(t *testing.T) {
	runner := &backendRunner{}
	backend := &Backend{
		Runner: runner,
		LookPath: func(string) (string, error) {
			return "", errors.New("not found")
		},
	}
	capability := backend.Probe(context.Background())
	if capability.ToolInstalled || capability.Status != "snapper_missing" {
		t.Fatalf("expected explicit missing-tool status: %+v", capability)
	}
	if len(runner.calls) != 0 {
		t.Fatalf("missing-tool probe must not run commands: %#v", runner.calls)
	}
}

func TestProbeOnlyListsSnapperConfiguration(t *testing.T) {
	runner := &backendRunner{responses: [][]byte{[]byte("Config | Subvolume\nroot | /\n")}}
	backend := &Backend{
		Runner:     runner,
		LookPath:   func(string) (string, error) { return "/usr/bin/snapper", nil },
		ConfigName: "root",
	}
	capability := backend.Probe(context.Background())
	if !capability.ConfigurationExists || !capability.CreationSupported {
		t.Fatalf("expected configured backend capability: %+v", capability)
	}
	if capability.Status != "configured_unverified" || capability.SupportsRollback ||
		!strings.Contains(capability.Reason, "not verified") {
		t.Fatalf("configured backend must not imply verified rollback: %+v", capability)
	}
	if len(runner.calls) != 1 || len(runner.calls[0]) != 2 ||
		runner.calls[0][0] != "snapper" || runner.calls[0][1] != "list-configs" {
		t.Fatalf("probe should only perform a read-only configuration listing: %#v", runner.calls)
	}
}

func TestCreateVerifiesEEDOwnedSnapshot(t *testing.T) {
	runner := &backendRunner{responses: [][]byte{
		[]byte("Config | Subvolume\nroot | /\n"),
		[]byte("42\n"),
		[]byte("Type | # | Pre # | Date | User | Cleanup | Description | Userdata\nsingle | 42 | | 2025-01-01T00:00:00Z | root | | Before EED | eed=true"),
	}}
	backend := &Backend{
		Runner:     runner,
		LookPath:   func(string) (string, error) { return "/usr/bin/snapper", nil },
		IsElevated: func() bool { return true },
		ConfigName: "root",
	}
	created, err := backend.Create(context.Background(), snapshot.CreateRequest{
		ConfigName:  "root",
		Description: "Before EED",
		ReadOnly:    true,
	})
	if err != nil {
		t.Fatal(err)
	}
	if created.Number != 42 || !created.EEDOwned || !created.ReadOnly {
		t.Fatalf("unexpected verified snapshot result: %+v", created)
	}
	if len(runner.calls) != 3 {
		t.Fatalf("expected config inspection, create, and post-create verification; got %d calls", len(runner.calls))
	}
	if runner.calls[1][1] != "-c" || runner.calls[1][2] != "root" || runner.calls[1][3] != "create" {
		t.Fatalf("unexpected snapper create arguments: %#v", runner.calls[1])
	}
}
