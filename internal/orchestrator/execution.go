package orchestrator

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	archpacman "eed/internal/adapters/arch/pacman"
	"eed/internal/configmanager"
	"eed/internal/servicemanager"
	"eed/internal/snapshot"
	"eed/internal/snapshot/snapper"
	"eed/internal/verification"
	"eed/internal/verification/checks"
)

type InstallOptions struct {
	DryRun           bool
	Confirmed        bool
	AllowUnprotected bool
	Input            io.Reader
	Output           io.Writer
	UseSudo          bool
	JournalStorage   *JournalStorage
	ConfigManager    *configmanager.ConfigManager
	ServiceManager   *servicemanager.ServiceManager
	PacmanEngine     *archpacman.TransactionEngine
	Runner           archpacman.Runner
}

// ExecuteInstallation orchestrates the end-to-end installation workflow.
func ExecuteInstallation(ctx context.Context, deps Dependencies, opts InstallOptions) (InstallationJournalRecord, error) {
	startedAt := time.Now().UTC()
	opID := fmt.Sprintf("eed-install-%d", startedAt.UnixNano())

	record := InstallationJournalRecord{
		SchemaVersion: "1",
		OperationID:   opID,
		StartedAt:     startedAt,
		OverallStatus: "failed",
		Errors:        []string{},
		Warnings:      []string{},
	}

	if opts.Output == nil {
		opts.Output = os.Stdout
	}
	if opts.JournalStorage == nil {
		opts.JournalStorage = DefaultJournalStorage()
	}

	// 1. Preflight: build complete preview
	preview, err := BuildPreview(ctx, deps)
	if err != nil {
		record.CompletedAt = time.Now().UTC()
		record.Errors = append(record.Errors, fmt.Sprintf("build preview: %v", err))
		_ = opts.JournalStorage.Save(record)
		return record, fmt.Errorf("build preview: %w", err)
	}

	record.Profile = preview.Profile
	record.HostOS = preview.Plan.HostOS
	record.TargetOS = preview.Plan.TargetOS
	record.SelectedComponents = append([]string(nil), preview.Selected...)

	// 2. Read-only preview / Dry-run check
	if opts.DryRun {
		record.CompletedAt = time.Now().UTC()
		record.OverallStatus = "dry_run"
		record.Warnings = append(record.Warnings, "dry-run mode: no system changes were made")
		_ = opts.JournalStorage.Save(record)
		return record, nil
	}

	// 3. Precondition: Host OS validation (MUST be Arch Linux)
	hostOS := strings.ToLower(strings.TrimSpace(preview.Plan.HostOS))
	if hostOS != "arch" && hostOS != "arch linux" {
		record.CompletedAt = time.Now().UTC()
		err := fmt.Errorf("installation blocked: current host operating system %q is not Arch Linux; Arch Linux installation engine must not execute on %s", preview.Plan.HostOS, preview.Plan.HostOS)
		record.Errors = append(record.Errors, err.Error())
		_ = opts.JournalStorage.Save(record)
		return record, err
	}

	if !preview.Plan.Installable {
		record.CompletedAt = time.Now().UTC()
		err := fmt.Errorf("installation blocked: plan is not installable on this host")
		record.Errors = append(record.Errors, err.Error())
		_ = opts.JournalStorage.Save(record)
		return record, err
	}

	if len(preview.Plan.Errors) > 0 {
		record.CompletedAt = time.Now().UTC()
		err := fmt.Errorf("installation blocked by plan errors: %s", strings.Join(preview.Plan.Errors, "; "))
		record.Errors = append(record.Errors, err.Error())
		_ = opts.JournalStorage.Save(record)
		return record, err
	}

	// 4. User confirmation
	if !opts.Confirmed {
		if opts.Input == nil {
			record.CompletedAt = time.Now().UTC()
			err := errors.New("installation requires explicit user confirmation (use interactive terminal or provide confirmation)")
			record.Errors = append(record.Errors, err.Error())
			_ = opts.JournalStorage.Save(record)
			return record, err
		}

		reader := bufio.NewReader(opts.Input)

		// Check snapshot capability
		if !preview.SnapshotFacts.SnapshotCreationSupported && !opts.AllowUnprotected {
			fmt.Fprintf(opts.Output, "\nWARNING: Snapshot protection is unavailable (%s).\n", preview.SnapshotFacts.Reason)
			fmt.Fprint(opts.Output, "Type PROCEED to continue without snapshot protection: ")
			line, readErr := reader.ReadString('\n')
			if readErr != nil || strings.TrimSpace(line) != "PROCEED" {
				record.CompletedAt = time.Now().UTC()
				err := errors.New("installation cancelled: user rejected unprotected installation")
				record.Errors = append(record.Errors, err.Error())
				_ = opts.JournalStorage.Save(record)
				return record, err
			}
		}

		// Prompt for overall confirmation
		fmt.Fprintln(opts.Output, "\nProposed Installation Summary:")
		fmt.Fprintf(opts.Output, "- Profile: %s\n", preview.Profile)
		fmt.Fprintf(opts.Output, "- Full System Upgrade: Pacman will synchronize repositories and perform a full system upgrade.\n")
		fmt.Fprintf(opts.Output, "- Package candidates: %s\n", strings.Join(preview.Changes.TransactionPackageCandidates, ", "))
		fmt.Fprint(opts.Output, "Type YES to authorize EED to start the installation: ")
		line, readErr := reader.ReadString('\n')
		if readErr != nil || strings.TrimSpace(line) != "YES" {
			record.CompletedAt = time.Now().UTC()
			err := errors.New("installation cancelled: transaction was not confirmed")
			record.Errors = append(record.Errors, err.Error())
			_ = opts.JournalStorage.Save(record)
			return record, err
		}
	}

	// 5. Snapshot Stage
	if preview.SnapshotFacts.SnapshotCreationSupported {
		snapperBackend := snapper.NewBackend()
		req := snapshot.CreateRequest{
			ConfigName:  "root",
			Description: fmt.Sprintf("EED installation %s", opID),
			Type:        "single",
			ReadOnly:    true,
			UserData:    map[string]string{"eed_operation": opID},
		}
		snap, snapErr := snapperBackend.Create(ctx, req)
		if snapErr == nil {
			record.SnapshotID = snap.Number
			record.SnapshotDescription = snap.Description
		} else {
			record.Warnings = append(record.Warnings, fmt.Sprintf("pre-install snapshot creation failed: %v", snapErr))
		}
	}

	// 6. Package Transaction Stage
	pkgTargets := preview.Changes.TransactionPackageCandidates
	if len(pkgTargets) > 0 {
		tx, txErr := archpacman.BuildTransaction(pkgTargets)
		if txErr != nil {
			record.CompletedAt = time.Now().UTC()
			record.Errors = append(record.Errors, fmt.Sprintf("build pacman transaction: %v", txErr))
			_ = opts.JournalStorage.Save(record)
			return record, txErr
		}

		approval, confErr := archpacman.ConfirmTransaction(tx, strings.NewReader("YES\n"), io.Discard)
		if confErr != nil {
			record.CompletedAt = time.Now().UTC()
			record.Errors = append(record.Errors, fmt.Sprintf("approve pacman transaction: %v", confErr))
			_ = opts.JournalStorage.Save(record)
			return record, confErr
		}

		pacmanEngine := opts.PacmanEngine
		if pacmanEngine == nil {
			archInspector := archpacman.NewInspector()
			if opts.Runner != nil {
				archInspector.Runner = opts.Runner
			}
			pacmanEngine = archpacman.NewTransactionEngine(opts.Runner, archInspector, preview.Plan.HostOS)
			pacmanEngine.UseSudo = opts.UseSudo || (opts.Runner == nil && os.Geteuid() != 0)
		}

		txRecord, execErr := pacmanEngine.Execute(ctx, tx, approval, preview.Selected)
		record.PacmanTransaction = &txRecord
		if execErr != nil {
			record.CompletedAt = time.Now().UTC()
			record.Errors = append(record.Errors, fmt.Sprintf("pacman transaction failed: %v", execErr))
			_ = opts.JournalStorage.Save(record)
			return record, execErr
		}
	}

	// 7. Configuration Deployment Stage
	cfgManager := opts.ConfigManager
	if cfgManager == nil {
		var cfgErr error
		cfgManager, cfgErr = configmanager.NewManager(configmanager.Options{})
		if cfgErr != nil {
			record.CompletedAt = time.Now().UTC()
			record.Errors = append(record.Errors, fmt.Sprintf("initialize config manager: %v", cfgErr))
			_ = opts.JournalStorage.Save(record)
			return record, cfgErr
		}
	}

	configItems := []configmanager.Item{}
	if preview.PortalPlan != nil && !preview.PortalPlan.PreserveExisting && preview.PortalPlan.Content != "" {
		configItems = append(configItems, configmanager.Item{
			Kind:        configmanager.KindFile,
			ComponentID: "system.portals",
			Destination: preview.PortalPlan.ConfigPath,
			Content:     []byte(preview.PortalPlan.Content),
		})
	}

	if len(configItems) > 0 {
		deployReport, deployErr := cfgManager.Deploy(ctx, opID, configItems, true)
		record.ConfigurationReport = &deployReport
		if deployErr != nil {
			record.CompletedAt = time.Now().UTC()
			record.Errors = append(record.Errors, fmt.Sprintf("configuration deployment failed: %v", deployErr))
			_ = cfgManager.Rollback(ctx, opID)
			_ = opts.JournalStorage.Save(record)
			return record, deployErr
		}
	}

	// 8. Service Configuration Stage
	svcManager := opts.ServiceManager
	if svcManager == nil {
		var svcErr error
		svcManager, svcErr = servicemanager.NewManager(servicemanager.Options{
			Runner:  opts.Runner,
			UseSudo: opts.UseSudo || (opts.Runner == nil && os.Geteuid() != 0),
		})
		if svcErr != nil {
			record.CompletedAt = time.Now().UTC()
			record.Errors = append(record.Errors, fmt.Sprintf("initialize service manager: %v", svcErr))
			_ = opts.JournalStorage.Save(record)
			return record, svcErr
		}
	}

	serviceRequests := collectServiceRequests(preview.Selected)
	if len(serviceRequests) > 0 {
		svcPlan, planErr := svcManager.Plan(ctx, serviceRequests)
		if planErr == nil {
			svcResult, applyErr := svcManager.Apply(ctx, opID, svcPlan, true, false)
			record.ServiceResult = &svcResult
			if applyErr != nil {
				record.Warnings = append(record.Warnings, fmt.Sprintf("service application warning: %v", applyErr))
			}
		} else {
			record.Warnings = append(record.Warnings, fmt.Sprintf("service planning warning: %v", planErr))
		}
	}

	// 9. Verification Stage
	verifierRegistry, regErr := checks.DefaultRegistry()
	if regErr == nil {
		cfgPath := deps.ConfigPath
		cfg, _ := deps.LoadConfig(cfgPath)
		detectReport, _ := deps.Detect(ctx)
		var vRunner verification.Runner = verification.ExecRunner{}
		if vr, ok := opts.Runner.(verification.Runner); ok {
			vRunner = vr
		}
		engine := verification.Engine{Registry: verifierRegistry}
		vReport, vErr := engine.Run(ctx, verification.Context{Detection: detectReport, Config: cfg, Runner: vRunner}, "")
		if vErr == nil {
			record.VerificationReport = &vReport
		}
	}

	record.CompletedAt = time.Now().UTC()
	record.OverallStatus = "success"
	if record.VerificationReport != nil && record.VerificationReport.OverallStatus == "completed_with_warnings" {
		record.OverallStatus = "completed_with_warnings"
	}

	_ = opts.JournalStorage.Save(record)
	return record, nil
}

func collectServiceRequests(selectedComponents []string) []servicemanager.ServiceRequest {
	var reqs []servicemanager.ServiceRequest
	for _, comp := range selectedComponents {
		switch comp {
		case "developer.docker":
			reqs = append(reqs, servicemanager.ServiceRequest{
				ComponentID: comp,
				UnitName:    "docker.service",
				Scope:       servicemanager.ScopeSystem,
				Action:      servicemanager.ActionEnableAndStart,
				Required:    false,
			})
		case "system.audio":
			reqs = append(reqs, servicemanager.ServiceRequest{
				ComponentID: comp,
				UnitName:    "pipewire.service",
				Scope:       servicemanager.ScopeUser,
				Action:      servicemanager.ActionEnableAndStart,
				Required:    false,
			})
		}
	}
	return reqs
}
