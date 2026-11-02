package verification

import (
	"context"
	"errors"
	"testing"

	"eed/internal/config"
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

func TestEngineRunsSelectedCheckAndAggregates(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(Check{
		ID:          "git.binary",
		ComponentID: "developer.git",
		Name:        "Git executable",
		Run: func(context.Context, Context) CheckResult {
			return CheckResult{Status: StatusPass}
		},
	}); err != nil {
		t.Fatal(err)
	}
	report, err := (Engine{Registry: registry}).Run(context.Background(), Context{Config: config.DefaultConfig()}, "")
	if err != nil {
		t.Fatal(err)
	}
	if report.Summary.Passed != 1 || report.OverallStatus != "completed" {
		t.Fatalf("unexpected report summary: %+v; status %s", report.Summary, report.OverallStatus)
	}
	if report.Components[0].Status != StatusPass {
		t.Fatalf("expected component PASS, got %s", report.Components[0].Status)
	}
}

func TestEngineSkipsUnselectedComponents(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(Check{
		ID:          "docker.binary",
		ComponentID: "developer.docker",
		Name:        "Docker executable",
		Run: func(context.Context, Context) CheckResult {
			t.Fatal("unselected check must not execute")
			return CheckResult{}
		},
	}); err != nil {
		t.Fatal(err)
	}
	report, err := (Engine{Registry: registry}).Run(context.Background(), Context{Config: config.DefaultConfig()}, "")
	if err != nil {
		t.Fatal(err)
	}
	if report.Summary.Skipped != 1 || report.Components[0].Checks[0].Status != StatusSkipped {
		t.Fatalf("expected skipped result, got %+v", report.Components[0])
	}
}

func TestEngineRejectsUnknownComponentFilter(t *testing.T) {
	registry := NewRegistry()
	if err := registry.Register(Check{
		ID:          "git.binary",
		ComponentID: "developer.git",
		Name:        "Git executable",
		Run:         func(context.Context, Context) CheckResult { return CheckResult{Status: StatusPass} },
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := (Engine{Registry: registry}).Run(context.Background(), Context{}, "missing.component"); err == nil {
		t.Fatal("expected unknown component filter error")
	}
}

func TestRegistryRejectsDuplicateCheck(t *testing.T) {
	registry := NewRegistry()
	check := Check{ID: "x", ComponentID: "recovery", Name: "Recovery", Run: func(context.Context, Context) CheckResult {
		return CheckResult{Status: StatusUnknown}
	}}
	if err := registry.Register(check); err != nil {
		t.Fatal(err)
	}
	if err := registry.Register(check); err == nil {
		t.Fatal("expected duplicate registration error")
	}
}
