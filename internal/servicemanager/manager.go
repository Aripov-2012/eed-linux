package servicemanager

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

type Runner interface {
	Run(context.Context, string, ...string) ([]byte, error)
}

type ExecRunner struct{}

func (ExecRunner) Run(ctx context.Context, name string, args ...string) ([]byte, error) {
	cmd := exec.CommandContext(ctx, name, args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C", "LANG=C")
	return cmd.CombinedOutput()
}

type Options struct {
	Runner      Runner
	LookPath    func(string) (string, error)
	InitSystem  string
	JournalPath string
	UseSudo     bool
}

type ServiceManager struct {
	Runner     Runner
	LookPath   func(string) (string, error)
	InitSystem string
	Journal    *Journal
	UseSudo    bool
}

func NewManager(opts Options) (*ServiceManager, error) {
	runner := opts.Runner
	if runner == nil {
		runner = ExecRunner{}
	}

	lookPath := opts.LookPath
	if lookPath == nil {
		lookPath = exec.LookPath
	}

	journalPath := opts.JournalPath
	if journalPath == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			home = os.Getenv("HOME")
		}
		journalPath = filepath.Join(home, ".local", "state", "eed", "service_journal.json")
	}

	journal, err := NewJournal(journalPath)
	if err != nil {
		return nil, fmt.Errorf("initialize service journal: %w", err)
	}

	return &ServiceManager{
		Runner:     runner,
		LookPath:   lookPath,
		InitSystem: opts.InitSystem,
		Journal:    journal,
		UseSudo:    opts.UseSudo || os.Geteuid() != 0,
	}, nil
}

// IsSystemdAvailable checks whether systemd and systemctl are accessible.
func (m *ServiceManager) IsSystemdAvailable(ctx context.Context) bool {
	if m.InitSystem != "" && !strings.EqualFold(m.InitSystem, "systemd") {
		return false
	}
	if _, err := m.LookPath("systemctl"); err != nil {
		return false
	}
	if _, err := os.Stat("/run/systemd/system"); err == nil {
		return true
	}
	// Fallback check
	out, err := m.Runner.Run(ctx, "systemctl", "is-system-running")
	if err == nil {
		return true
	}
	state := strings.TrimSpace(string(out))
	return state == "running" || state == "degraded" || state == "starting"
}

// Inspect queries current load, active, and enabled state for a unit.
func (m *ServiceManager) Inspect(ctx context.Context, unitName string, scope Scope) (ServiceState, error) {
	state := ServiceState{
		UnitName: unitName,
		Scope:    scope,
	}

	args := []string{}
	if scope == ScopeUser {
		args = append(args, "--user")
	} else {
		args = append(args, "--system")
	}
	args = append(args, "show", "--property=Id,LoadState,ActiveState,UnitFileState", "--", unitName)

	out, err := m.Runner.Run(ctx, "systemctl", args...)
	if err != nil {
		state.ActiveState = "unavailable"
		state.UnitFileState = "unavailable"
		return state, fmt.Errorf("systemctl show for %s: %s", unitName, commandFailure(out, err))
	}

	props := parseProperties(string(out))
	loadState := props["LoadState"]
	activeState := props["ActiveState"]
	unitFileState := props["UnitFileState"]

	state.Loaded = loadState == "loaded"
	state.Exists = state.Loaded || (loadState != "not-found" && loadState != "")
	state.ActiveState = activeState
	state.UnitFileState = unitFileState
	state.Active = activeState == "active"
	state.Enabled = unitFileState == "enabled" || unitFileState == "static" || unitFileState == "alias"

	return state, nil
}

// Plan prepares proposed service changes and verifies unit existence before execution.
func (m *ServiceManager) Plan(ctx context.Context, requests []ServiceRequest) (ServicePlan, error) {
	plan := ServicePlan{
		SystemdAvailable: m.IsSystemdAvailable(ctx),
		ProposedChanges:  []ProposedChange{},
		Warnings:         []string{},
		Errors:           []string{},
	}

	if !plan.SystemdAvailable {
		plan.Warnings = append(plan.Warnings, "systemd is not available or not the active init system; service management is disabled")
		return plan, nil
	}

	for _, req := range requests {
		if strings.TrimSpace(req.UnitName) == "" {
			continue
		}

		current, err := m.Inspect(ctx, req.UnitName, req.Scope)
		if err != nil {
			if req.Required {
				plan.Errors = append(plan.Errors, fmt.Sprintf("service %s check failed: %v", req.UnitName, err))
			} else {
				plan.Warnings = append(plan.Warnings, fmt.Sprintf("optional service %s check failed: %v", req.UnitName, err))
			}
			continue
		}

		if !current.Exists {
			if req.Required {
				plan.Errors = append(plan.Errors, fmt.Sprintf("required service unit %s does not exist", req.UnitName))
			} else {
				plan.Warnings = append(plan.Warnings, fmt.Sprintf("optional service unit %s does not exist; skipping", req.UnitName))
			}
			continue
		}

		change := ProposedChange{
			ComponentID: req.ComponentID,
			UnitName:    req.UnitName,
			Scope:       req.Scope,
			Action:      req.Action,
			BeforeState: current,
			WillMutate:  false,
		}

		needsEnable := (req.Action == ActionEnable || req.Action == ActionEnableAndStart) && !current.Enabled
		needsStart := (req.Action == ActionStart || req.Action == ActionEnableAndStart) && !current.Active

		if needsEnable || needsStart {
			change.WillMutate = true
			change.Command = m.buildCommand(req.UnitName, req.Scope, req.Action)
		}

		plan.ProposedChanges = append(plan.ProposedChanges, change)
	}

	return plan, nil
}

// Apply executes proposed service mutations. When dryRun is true, NO system changes are made.
func (m *ServiceManager) Apply(ctx context.Context, opID string, plan ServicePlan, approved bool, dryRun bool) (ServiceResult, error) {
	if opID == "" {
		opID = fmt.Sprintf("svc-%d", time.Now().UnixNano())
	}

	result := ServiceResult{
		OperationID: opID,
		Timestamp:   time.Now().UTC(),
		Applied:     []JournalEntry{},
		Simulated:   dryRun,
		Success:     false,
		Errors:      []string{},
		Warnings:    []string{},
	}

	if !plan.SystemdAvailable {
		result.Success = true
		result.Warnings = append(result.Warnings, "systemd unavailable; no service changes made")
		return result, nil
	}

	if dryRun {
		result.Success = true
		for _, change := range plan.ProposedChanges {
			if change.WillMutate {
				result.Applied = append(result.Applied, JournalEntry{
					OperationID: opID,
					Timestamp:   time.Now().UTC(),
					ComponentID: change.ComponentID,
					UnitName:    change.UnitName,
					Scope:       change.Scope,
					Action:      change.Action,
					Status:      "simulated",
				})
			}
		}
		return result, nil
	}

	if !approved {
		err := errors.New("service configuration changes require explicit user approval")
		result.Errors = append(result.Errors, err.Error())
		return result, err
	}

	for _, change := range plan.ProposedChanges {
		if !change.WillMutate {
			continue
		}

		if err := ctx.Err(); err != nil {
			result.Errors = append(result.Errors, err.Error())
			return result, err
		}

		cmdArgs := change.Command
		if len(cmdArgs) == 0 {
			continue
		}

		out, err := m.Runner.Run(ctx, cmdArgs[0], cmdArgs[1:]...)
		if err != nil {
			errDetail := fmt.Sprintf("apply service action %s on %s: %s", change.Action, change.UnitName, commandFailure(out, err))
			result.Errors = append(result.Errors, errDetail)
			return result, errors.New(errDetail)
		}

		entry := JournalEntry{
			OperationID: opID,
			Timestamp:   time.Now().UTC(),
			ComponentID: change.ComponentID,
			UnitName:    change.UnitName,
			Scope:       change.Scope,
			Action:      change.Action,
			Status:      "applied",
		}
		entry.Previous.Active = change.BeforeState.Active
		entry.Previous.Enabled = change.BeforeState.Enabled
		entry.Previous.State = change.BeforeState.ActiveState

		m.Journal.AddEntry(entry)
		result.Applied = append(result.Applied, entry)
	}

	if err := m.Journal.Save(); err != nil {
		result.Warnings = append(result.Warnings, fmt.Sprintf("save service journal: %v", err))
	}

	result.Success = true
	return result, nil
}

// Rollback reverses applied service changes recorded in the journal.
func (m *ServiceManager) Rollback(ctx context.Context, opID string) error {
	entries := m.Journal.EntriesByOperation(opID)
	if len(entries) == 0 {
		return fmt.Errorf("no service journal entries found for operation ID %q", opID)
	}

	for i := len(entries) - 1; i >= 0; i-- {
		if err := ctx.Err(); err != nil {
			return err
		}

		entry := entries[i]
		if entry.Status == "rolled_back" {
			continue
		}

		// If service was started by EED and wasn't active previously, stop it
		if (entry.Action == ActionStart || entry.Action == ActionEnableAndStart) && !entry.Previous.Active {
			cmd := m.buildCommand(entry.UnitName, entry.Scope, ActionStop)
			if len(cmd) > 0 {
				_, _ = m.Runner.Run(ctx, cmd[0], cmd[1:]...)
			}
		}

		// If service was enabled by EED and wasn't enabled previously, disable it
		if (entry.Action == ActionEnable || entry.Action == ActionEnableAndStart) && !entry.Previous.Enabled {
			cmd := m.buildCommand(entry.UnitName, entry.Scope, ActionDisable)
			if len(cmd) > 0 {
				_, _ = m.Runner.Run(ctx, cmd[0], cmd[1:]...)
			}
		}

		m.Journal.UpdateStatus(opID, entry.UnitName, entry.Scope, "rolled_back")
	}

	return m.Journal.Save()
}

func (m *ServiceManager) buildCommand(unitName string, scope Scope, action Action) []string {
	var cmd []string
	isSystem := scope == ScopeSystem

	if isSystem && m.UseSudo && os.Geteuid() != 0 {
		cmd = append(cmd, "sudo", "systemctl", "--system")
	} else if isSystem {
		cmd = append(cmd, "systemctl", "--system")
	} else {
		cmd = append(cmd, "systemctl", "--user")
	}

	switch action {
	case ActionEnable:
		cmd = append(cmd, "enable", unitName)
	case ActionStart:
		cmd = append(cmd, "start", unitName)
	case ActionEnableAndStart:
		cmd = append(cmd, "enable", "--now", unitName)
	case ActionDisable:
		cmd = append(cmd, "disable", unitName)
	case ActionStop:
		cmd = append(cmd, "stop", unitName)
	default:
		return nil
	}

	return cmd
}

func parseProperties(output string) map[string]string {
	props := make(map[string]string)
	for _, line := range strings.Split(output, "\n") {
		if k, v, ok := strings.Cut(line, "="); ok {
			props[strings.TrimSpace(k)] = strings.TrimSpace(v)
		}
	}
	return props
}

func commandFailure(output []byte, err error) string {
	detail := strings.TrimSpace(string(output))
	if detail == "" {
		return err.Error()
	}
	return detail
}
