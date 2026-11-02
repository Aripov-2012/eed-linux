package checks

import (
	"context"
	"errors"
	"testing"

	"eed/internal/config"
	"eed/internal/verification"
)

type testRunner struct {
	paths   map[string]string
	outputs map[string][]byte
	errors  map[string]error
}

func (r testRunner) LookPath(name string) (string, error) {
	path, ok := r.paths[name]
	if !ok {
		return "", errors.New("not found")
	}
	return path, nil
}

func (r testRunner) Run(_ context.Context, name string, _ ...string) ([]byte, error) {
	if err := r.errors[name]; err != nil {
		return r.outputs[name], err
	}
	return r.outputs[name], nil
}

func TestSelectedExecutableCheckReportsMissingBinary(t *testing.T) {
	registry, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	report, err := (verification.Engine{Registry: registry}).Run(context.Background(), verification.Context{
		Config: config.DefaultConfig(),
		Runner: testRunner{paths: map[string]string{}, outputs: map[string][]byte{}, errors: map[string]error{}},
	}, "developer.git")
	if err != nil {
		t.Fatal(err)
	}
	if report.Summary.Failed != 1 || report.Components[0].Checks[0].Status != verification.StatusFail {
		t.Fatalf("expected selected missing binary to fail, got %+v", report)
	}
}

func TestUnselectedPackageChecksAreSkipped(t *testing.T) {
	registry, err := DefaultRegistry()
	if err != nil {
		t.Fatal(err)
	}
	report, err := (verification.Engine{Registry: registry}).Run(context.Background(), verification.Context{Config: config.DefaultConfig()}, "developer.docker")
	if err != nil {
		t.Fatal(err)
	}
	if report.Summary.Skipped != 2 {
		t.Fatalf("expected Docker binary and daemon checks to be skipped, got %+v", report.Summary)
	}
}
