package cli

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	builtin "eed"
	archpacman "eed/internal/adapters/arch/pacman"
	archportals "eed/internal/adapters/arch/portals"
	debianapt "eed/internal/adapters/debian/apt"
	"eed/internal/assessment"
	"eed/internal/components"
	eedconfig "eed/internal/config"
	"eed/internal/detection"
	"eed/internal/orchestrator"
	"eed/internal/packageinfo"
	"eed/internal/planner"
	"eed/internal/snapshot"
	"eed/internal/snapshot/snapper"
	"eed/internal/storage"
	"eed/internal/testdrive"
	"eed/internal/verification"
	"eed/internal/verification/checks"
	"eed/internal/version"
	"eed/internal/wizard"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/pelletier/go-toml/v2"
	"github.com/spf13/cobra"
)

func Execute() error {
	return NewRootCmd().Execute()
}

func NewRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:           "eed",
		Short:         "EED — Eliminate Endless Torture",
		Long:          "Modular Arch Linux configuration and automation platform.",
		Args:          cobra.ArbitraryArgs,
		SilenceUsage:  true,
		SilenceErrors: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return cmd.Help()
		},
	}
	root.PersistentFlags().String("data-dir", "", "component data directory (overrides EED_DATA_DIR and built-in manifests)")

	root.AddCommand(NewVersionCmd())
	root.AddCommand(NewWizardCmd())
	root.AddCommand(NewConfigCmd())
	root.AddCommand(NewDetectCmd())
	root.AddCommand(NewStorageCmd())
	root.AddCommand(NewSnapshotCmd())
	root.AddCommand(NewVerifyCmd())
	root.AddCommand(NewDoctorCmd())
	root.AddCommand(NewInstallCmd())
	root.AddCommand(NewTestDriveCmd())
	root.AddCommand(NewPlanCmd())
	root.AddCommand(NewAssessCmd())
	return root
}

func NewVersionCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "version",
		Short: "Print EED version information",
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonOutput, err := cmd.Flags().GetBool("json")
			if err != nil {
				return fmt.Errorf("read json flag: %w", err)
			}
			if jsonOutput {
				encoder := json.NewEncoder(cmd.OutOrStdout())
				encoder.SetIndent("", "  ")
				return encoder.Encode(version.Details())
			}
			fmt.Fprintln(cmd.OutOrStdout(), version.Details().String())
			return nil
		},
	}
	cmd.Flags().Bool("json", false, "output version metadata as JSON")
	return cmd
}

func NewWizardCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "wizard",
		Short: "Run the interactive configuration wizard",
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := eedconfig.ResolveConfigPath()
			if err != nil {
				return fmt.Errorf("resolve config path: %w", err)
			}

			model := wizard.NewModel(path)
			program := tea.NewProgram(model)
			finalModel, err := program.Run()
			if err != nil {
				return fmt.Errorf("run wizard: %w", err)
			}

			completed, ok := finalModel.(wizard.Model)
			if !ok {
				return fmt.Errorf("unexpected wizard result type: %T", finalModel)
			}
			if !completed.Saved {
				fmt.Fprintln(cmd.OutOrStdout(), "Wizard cancelled; no changes were saved.")
				return nil
			}
			if err := completed.Persist(); err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Configuration saved to %s\n", path)
			return nil
		},
	}
	return cmd
}

func NewDetectCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "detect",
		Short: "Collect a read-only system detection report",
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonOutput, err := cmd.Flags().GetBool("json")
			if err != nil {
				return fmt.Errorf("read json flag: %w", err)
			}
			detailed, err := cmd.Flags().GetBool("detailed")
			if err != nil {
				return fmt.Errorf("read detailed flag: %w", err)
			}

			report, err := detection.Detect(context.Background())
			if err != nil {
				return err
			}

			if jsonOutput {
				encoder := json.NewEncoder(cmd.OutOrStdout())
				encoder.SetIndent("", "  ")
				return encoder.Encode(report)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "OS: %s\n", report.System.OS.Name)
			if report.System.OS.ID != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Distribution: %s (%s)\n", report.System.OS.Name, report.System.OS.ID)
			}
			if report.System.OS.Version != "" || report.System.OS.VersionCodename != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Release: %s (%s)\n", valueOrUnknown(report.System.OS.Version), valueOrUnknown(report.System.OS.VersionCodename))
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Kernel: %s\n", report.System.Kernel.Release)
			fmt.Fprintf(cmd.OutOrStdout(), "Architecture: %s\n", report.System.Architecture)
			fmt.Fprintf(cmd.OutOrStdout(), "Detected desktop: %s\n", report.System.Desktop)
			fmt.Fprintf(cmd.OutOrStdout(), "Session: %s\n", report.System.SessionType)
			fmt.Fprintf(cmd.OutOrStdout(), "Display manager: %s\n", valueOrUnknown(report.System.DisplayManager))
			fmt.Fprintf(cmd.OutOrStdout(), "CPU: %s (%d logical / %d physical)\n", report.Hardware.CPU.Model, report.Hardware.CPU.LogicalCores, report.Hardware.CPU.PhysicalCores)
			fmt.Fprintf(cmd.OutOrStdout(), "Memory: total=%s bytes, available=%s bytes\n",
				knownBytes(report.Hardware.Memory.TotalBytes, report.Hardware.Memory.TotalBytes > 0),
				knownBytes(report.Hardware.Memory.AvailableBytes, report.Hardware.Memory.AvailableBytes > 0))
			fmt.Fprintln(cmd.OutOrStdout(), "GPUs:")
			if len(report.Hardware.GPUs) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "  (not detected)")
			}
			for _, gpu := range report.Hardware.GPUs {
				driverState := gpuDriverDescription(gpu.Driver, gpu.DriverBound)
				fmt.Fprintf(cmd.OutOrStdout(), "  - %s %s (PCI vendor %s, device %s; %s)\n",
					gpu.Vendor, gpu.Model, gpu.VendorID, gpu.DeviceID, driverState)
			}
			printDeviceSummary(cmd, report, detailed)
			printMountSummary(cmd, report, detailed)
			fmt.Fprintf(cmd.OutOrStdout(), "Init system: %s (systemd available: %t)\n",
				valueOrUnknown(report.System.InitSystem), report.Software.SystemdAvailable)
			fmt.Fprintln(cmd.OutOrStdout(), "Services:")
			if len(report.Software.Services) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "  (not checked or none detected)")
			}
			for _, service := range report.Software.Services {
				status := "not installed"
				if service.Installed {
					status = "installed"
				}
				if service.Loaded {
					status += ", loaded"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "  - %s (%s): %s (%s)\n",
					service.Name, service.Scope, valueOrUnknown(service.State), status)
			}
			hostDistribution, targetDistribution, targetManager := "unknown", "unknown", "unknown"
			hostNativeManager := "unknown"
			if len(report.Software.PackageManagers) > 0 {
				hostDistribution = report.Software.PackageManagers[0].HostDistribution
				targetDistribution = report.Software.PackageManagers[0].ConfiguredTargetDistribution
				targetManager = report.Software.PackageManagers[0].TargetPackageManager
				for _, manager := range report.Software.PackageManagers {
					if manager.HostNative {
						hostNativeManager = manager.Name
						break
					}
				}
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Host distribution: %s\nHost-native package manager: %s\n",
				hostDistribution, hostNativeManager)
			fmt.Fprintf(cmd.OutOrStdout(), "Configured target distribution: %s\nTarget package manager: %s\n",
				targetDistribution, targetManager)
			fmt.Fprintln(cmd.OutOrStdout(), "Package managers:")
			for _, manager := range report.Software.PackageManagers {
				state := "unavailable"
				if manager.Available {
					state = "available"
				}
				version := manager.Version
				if version == "" {
					version = "version unavailable"
				}
				role := "not host-native"
				if manager.HostNative {
					role = "host-native"
				} else if manager.Name == manager.TargetPackageManager {
					role = "target manager"
				}
				fmt.Fprintf(cmd.OutOrStdout(), "  - %s: %s, %s, adapter %s, %s\n",
					manager.Name, state, role, manager.AdapterStatus, version)
			}
			return nil
		},
	}
	cmd.Flags().Bool("json", false, "output the detection report as JSON")
	cmd.Flags().Bool("detailed", false, "show every block device and mounted filesystem")
	return cmd
}

func printDeviceSummary(cmd *cobra.Command, report detection.Report, detailed bool) {
	out := cmd.OutOrStdout()
	fmt.Fprintln(out, "Physical disks:")
	diskCount := 0
	for _, device := range report.System.BlockDevices {
		if device.Kind != "physical_disk" && (device.Kind != "" || device.IsPartition) {
			continue
		}
		if device.IsPartition {
			continue
		}
		diskCount++
		transport := ""
		if device.Transport != "" && device.Transport != "unknown" {
			transport = ", " + device.Transport
		}
		fmt.Fprintf(out, "  - %s: %s%s\n", device.Name, formatSize(device.SizeBytes, device.SizeKnown), transport)
	}
	if diskCount == 0 {
		fmt.Fprintln(out, "  (not detected)")
	}
	fmt.Fprintln(out, "Partitions:")
	partitionCount := 0
	for _, device := range report.System.BlockDevices {
		if !device.IsPartition && device.Kind != "partition" {
			continue
		}
		partitionCount++
		description := formatSize(device.SizeBytes, device.SizeKnown)
		for _, mount := range report.System.Storage {
			if mount.Major == device.Major && mount.Minor == device.Minor {
				description += ", " + mount.Filesystem
				if mount.Category == "efi" {
					description += " (EFI)"
				}
				description += ", mounted at " + mount.MountPoint
				break
			}
		}
		fmt.Fprintf(out, "  - %s: %s\n", device.Name, description)
	}
	if partitionCount == 0 {
		fmt.Fprintln(out, "  (not detected)")
	}
	if detailed {
		fmt.Fprintln(out, "All block devices:")
		for _, device := range report.System.BlockDevices {
			fmt.Fprintf(out, "  - %s: %s (%s, %s)\n", device.Name, device.Path,
				valueOrUnknown(device.Kind), formatSize(device.SizeBytes, device.SizeKnown))
		}
		return
	}
	loopCount, mapperCount, virtualCount := 0, 0, 0
	for _, device := range report.System.BlockDevices {
		switch device.Kind {
		case "loop":
			loopCount++
		case "device_mapper":
			mapperCount++
		case "virtual":
			virtualCount++
		}
	}
	if loopCount+mapperCount+virtualCount > 0 {
		fmt.Fprintln(out, "Virtual devices:")
		if loopCount > 0 {
			fmt.Fprintf(out, "  - Loop devices: %d (primarily Snap)\n", loopCount)
		}
		if mapperCount > 0 {
			fmt.Fprintf(out, "  - Device-mapper devices: %d\n", mapperCount)
		}
		if virtualCount > 0 {
			fmt.Fprintf(out, "  - Other virtual devices: %d\n", virtualCount)
		}
	}
}

func printMountSummary(cmd *cobra.Command, report detection.Report, detailed bool) {
	out := cmd.OutOrStdout()
	fmt.Fprintln(out, "Mounted filesystems:")
	if len(report.System.Storage) == 0 {
		fmt.Fprintln(out, "  (not detected)")
		return
	}
	if detailed {
		for _, mount := range report.System.Storage {
			fmt.Fprintf(out, "  - %s: %s on %s, total=%s, available=%s [%s]\n",
				mount.SourceDevice, mount.Filesystem, mount.MountPoint,
				formatSize(mount.TotalBytes, mount.CapacityKnown),
				formatSize(mount.AvailableBytes, mount.CapacityKnown), valueOrUnknown(mount.Category))
		}
		return
	}

	grouped := map[string]int{}
	for _, mount := range report.System.Storage {
		switch mount.Category {
		case "root", "home", "bind", "boot", "efi":
			name := mount.Category
			if name == "bind" {
				name = "home"
			}
			bindNote := ""
			if mount.BindMount {
				bindNote = " (bind mount; same filesystem)"
			}
			label := strings.ToUpper(name[:1]) + name[1:]
			if mount.Category == "efi" {
				label = "EFI"
			}
			fmt.Fprintf(out, "  - %s: %s on %s, available=%s\n",
				label, mount.Filesystem, mount.MountPoint,
				formatSize(mount.AvailableBytes, mount.CapacityKnown))
			if bindNote != "" {
				fmt.Fprintf(out, "    %s\n", strings.TrimSpace(bindNote))
			}
		case "snap", "pseudo", "overlay", "fuse", "namespace", "virtual":
			grouped[mount.Category]++
		default:
			fmt.Fprintf(out, "  - %s: %s on %s, available=%s\n",
				mount.SourceDevice, mount.Filesystem, mount.MountPoint,
				formatSize(mount.AvailableBytes, mount.CapacityKnown))
		}
	}
	for _, category := range []string{"snap", "pseudo", "overlay", "fuse", "namespace"} {
		if grouped[category] > 0 {
			label := map[string]string{
				"snap": "Snap squashfs mounts", "pseudo": "Pseudo filesystems",
				"overlay": "Docker/container overlays", "fuse": "FUSE/GVFS mounts",
				"namespace": "Namespace mounts",
				"virtual":   "Other virtual filesystems",
			}[category]
			fmt.Fprintf(out, "  - %s: %d (use --detailed to list)\n", label, grouped[category])
		}
	}
}

func formatSize(bytes int64, known bool) string {
	if !known {
		return "unknown"
	}
	size := float64(bytes) / (1024 * 1024 * 1024)
	if size < 10 {
		return fmt.Sprintf("%.1f GiB", size)
	}
	return fmt.Sprintf("%.0f GiB", size)
}

func gpuDriverDescription(driver string, bound bool) string {
	driver = strings.TrimSpace(driver)
	if !bound || driver == "" {
		return "no bound driver detected"
	}
	return "driver " + driver + " bound"
}

func valueOrUnknown(value string) string {
	if strings.TrimSpace(value) == "" {
		return "unknown"
	}
	return value
}

func knownBytes(value int64, known bool) string {
	if !known {
		return "unknown"
	}
	return fmt.Sprintf("%d", value)
}

func NewPlanCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "plan",
		Short: "Generate a non-destructive installation plan from config and detection data",
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonOutput, err := cmd.Flags().GetBool("json")
			if err != nil {
				return fmt.Errorf("read json flag: %w", err)
			}
			showAssessment, err := cmd.Flags().GetBool("assessment")
			if err != nil {
				return fmt.Errorf("read assessment flag: %w", err)
			}

			cfgPath, err := eedconfig.ResolveConfigPath()
			if err != nil {
				return fmt.Errorf("resolve config path: %w", err)
			}
			cfg, err := eedconfig.LoadConfig(cfgPath)
			if err != nil {
				return err
			}

			dataDir, err := readDataDirFlag(cmd)
			if err != nil {
				return err
			}
			registry, err := components.LoadDiscovered(dataDir, builtin.BuiltinManifests())
			if err != nil {
				return fmt.Errorf("load component manifests: %w", err)
			}

			report, err := detection.Detect(context.Background())
			if err != nil {
				return fmt.Errorf("detect host state: %w", err)
			}

			plan, err := planner.BuildPlan(cfg, registry, report)
			if err != nil {
				return err
			}

			if jsonOutput {
				encoder := json.NewEncoder(cmd.OutOrStdout())
				encoder.SetIndent("", "  ")
				return encoder.Encode(plan)
			}
			fmt.Fprint(cmd.OutOrStdout(), planner.FormatPlan(plan))
			if showAssessment {
				res, err := assessment.Evaluate(cfg, plan, registry, report)
				if err != nil {
					return err
				}
				fmt.Fprintf(cmd.OutOrStdout(), "\nAssessment: %s\n", res.OverallStatus)
				if len(res.Warnings) > 0 {
					fmt.Fprintln(cmd.OutOrStdout(), "Warnings:")
					for _, warning := range res.Warnings {
						fmt.Fprintf(cmd.OutOrStdout(), "  - %s\n", warning)
					}
				}
			}
			return nil
		},
	}
	cmd.Flags().Bool("json", false, "output the plan as JSON")
	cmd.Flags().Bool("assessment", false, "include the compatibility and resource assessment summary")
	return cmd
}

func NewAssessCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "assess",
		Short: "Evaluate compatibility and resource readiness for the current configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonOutput, err := cmd.Flags().GetBool("json")
			if err != nil {
				return fmt.Errorf("read json flag: %w", err)
			}

			cfgPath, err := eedconfig.ResolveConfigPath()
			if err != nil {
				return fmt.Errorf("resolve config path: %w", err)
			}
			cfg, err := eedconfig.LoadConfig(cfgPath)
			if err != nil {
				return err
			}

			dataDir, err := readDataDirFlag(cmd)
			if err != nil {
				return err
			}
			registry, err := components.LoadDiscovered(dataDir, builtin.BuiltinManifests())
			if err != nil {
				return fmt.Errorf("load component manifests: %w", err)
			}

			report, err := detection.Detect(context.Background())
			if err != nil {
				return fmt.Errorf("detect host state: %w", err)
			}

			plan, err := planner.BuildPlan(cfg, registry, report)
			if err != nil {
				return err
			}

			assessmentReport, err := assessment.Evaluate(cfg, plan, registry, report)
			if err != nil {
				return err
			}

			if jsonOutput {
				encoder := json.NewEncoder(cmd.OutOrStdout())
				encoder.SetIndent("", "  ")
				return encoder.Encode(assessmentReport)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Overall status: %s\n", assessmentReport.OverallStatus)
			fmt.Fprintf(cmd.OutOrStdout(), "Compatibility: %s\n", assessmentReport.Compatibility.Status)
			fmt.Fprintf(cmd.OutOrStdout(), "Resources: %s\n", assessmentReport.Resources.Status)
			ram := assessmentReport.Resources.RAM
			fmt.Fprintf(cmd.OutOrStdout(), "RAM: minimum total=%s bytes (%s), recommended total=%s bytes (%s), minimum available=%s bytes (%s)\n",
				knownBytes(ram.MinimumTotalBytes, ram.MinimumTotalBytes > 0),
				ram.TotalStatus,
				knownBytes(ram.RecommendedTotalBytes, ram.RecommendedTotalBytes > 0),
				ram.RecommendedStatus,
				knownBytes(ram.MinimumAvailableBytes, ram.MinimumAvailableBytes > 0),
				ram.AvailableStatus)
			if len(assessmentReport.Warnings) > 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "Warnings:")
				for _, warning := range assessmentReport.Warnings {
					fmt.Fprintf(cmd.OutOrStdout(), "  - %s\n", warning)
				}
			}
			if len(assessmentReport.Errors) > 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "Errors:")
				for _, err := range assessmentReport.Errors {
					fmt.Fprintf(cmd.OutOrStdout(), "  - %s\n", err)
				}
			}
			return nil
		},
	}
	cmd.Flags().Bool("json", false, "output the assessment as JSON")
	return cmd
}

func NewStorageCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "storage",
		Short: "Analyze filesystems and storage capacity in read-only mode",
	}
	cmd.AddCommand(NewStorageAnalyzeCmd())
	return cmd
}

func NewStorageAnalyzeCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "analyze",
		Short: "Collect a read-only storage analysis report",
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonOutput, err := cmd.Flags().GetBool("json")
			if err != nil {
				return fmt.Errorf("read json flag: %w", err)
			}
			report, err := storage.Analyze()
			if err != nil {
				return err
			}
			if jsonOutput {
				encoder := json.NewEncoder(cmd.OutOrStdout())
				encoder.SetIndent("", "  ")
				return encoder.Encode(report)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Root filesystem: %s (%s)\n", report.Summary.RootFilesystem, report.Summary.RootMountPoint)
			fmt.Fprintf(cmd.OutOrStdout(), "Home filesystem: %s (%s)\n", report.Summary.HomeFilesystem, report.Summary.HomeMountPoint)
			fmt.Fprintf(cmd.OutOrStdout(), "Snapshot backend candidate: %s\n", report.Summary.SnapshotBackendCandidate)
			for _, fs := range report.Filesystems {
				fmt.Fprintf(cmd.OutOrStdout(), "- %s on %s: %s (%d bytes used)\n", fs.MountPoint, fs.SourceDevice, fs.Filesystem, fs.UsedBytes)
			}
			if len(report.Warnings) > 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "Warnings:")
				for _, warning := range report.Warnings {
					fmt.Fprintf(cmd.OutOrStdout(), "  - %s\n", warning)
				}
			}
			return nil
		},
	}
	cmd.Flags().Bool("json", false, "output the storage report as JSON")
	return cmd
}

func NewSnapshotCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "snapshot",
		Short: "Inspect and create read-only system snapshots through an existing Snapper backend",
	}
	cmd.AddCommand(NewSnapshotStatusCmd())
	cmd.AddCommand(NewSnapshotListCmd())
	cmd.AddCommand(NewSnapshotCreateCmd())
	return cmd
}

func NewSnapshotStatusCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "status",
		Short: "Probe the available snapshot backend and readiness status",
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonOutput, err := cmd.Flags().GetBool("json")
			if err != nil {
				return fmt.Errorf("read json flag: %w", err)
			}
			backend := snapper.NewBackend()
			capability := backend.Probe(context.Background())
			if jsonOutput {
				encoder := json.NewEncoder(cmd.OutOrStdout())
				encoder.SetIndent("", "  ")
				return encoder.Encode(capability)
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Backend: %s\n", capability.Backend)
			fmt.Fprintf(cmd.OutOrStdout(), "Status: %s\n", capability.Status)
			fmt.Fprintf(cmd.OutOrStdout(), "Available: %t\n", capability.Available)
			fmt.Fprintf(cmd.OutOrStdout(), "Tool installed: %t\n", capability.ToolInstalled)
			fmt.Fprintf(cmd.OutOrStdout(), "Configuration exists: %t\n", capability.ConfigurationExists)
			fmt.Fprintf(cmd.OutOrStdout(), "Creation supported: %t\n", capability.CreationSupported)
			fmt.Fprintf(cmd.OutOrStdout(), "Rollback supported: %t\n", capability.SupportsRollback)
			if capability.Reason != "" {
				fmt.Fprintf(cmd.OutOrStdout(), "Reason: %s\n", capability.Reason)
			}
			return nil
		},
	}
	cmd.Flags().Bool("json", false, "output the snapshot capability report as JSON")
	return cmd
}

func NewSnapshotListCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List snapshots from the configured Snapper backend",
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonOutput, err := cmd.Flags().GetBool("json")
			if err != nil {
				return fmt.Errorf("read json flag: %w", err)
			}
			configName, err := cmd.Flags().GetString("config")
			if err != nil {
				return fmt.Errorf("read config flag: %w", err)
			}
			backend := snapper.NewBackend()
			if configName != "" {
				backend.ConfigName = configName
			}
			entries, err := backend.List(context.Background(), backend.ConfigName)
			if err != nil {
				return err
			}
			if jsonOutput {
				encoder := json.NewEncoder(cmd.OutOrStdout())
				encoder.SetIndent("", "  ")
				return encoder.Encode(entries)
			}
			if len(entries) == 0 {
				fmt.Fprintln(cmd.OutOrStdout(), "No snapshots found.")
				return nil
			}
			for _, item := range entries {
				fmt.Fprintf(cmd.OutOrStdout(), "#%d %s %s\n", item.Number, item.Type, item.Description)
			}
			return nil
		},
	}
	cmd.Flags().Bool("json", false, "output snapshots as JSON")
	cmd.Flags().String("config", "root", "Snapper configuration name to inspect")
	return cmd
}

func NewSnapshotCreateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "create",
		Short: "Create an EED-owned read-only snapshot via Snapper",
		RunE: func(cmd *cobra.Command, args []string) error {
			configName, err := cmd.Flags().GetString("config")
			if err != nil {
				return fmt.Errorf("read config flag: %w", err)
			}
			description, err := cmd.Flags().GetString("description")
			if err != nil {
				return fmt.Errorf("read description flag: %w", err)
			}
			if description == "" {
				description = "Before EED operation"
			}
			backend := snapper.NewBackend()
			if configName != "" {
				backend.ConfigName = configName
			}
			if backend.ConfigName == "" {
				return fmt.Errorf("snapshot configuration is required; use --config or run via sudo eed snapshot create --config <name>")
			}
			created, err := backend.Create(context.Background(), snapshot.CreateRequest{ConfigName: backend.ConfigName, Description: description, Type: "single", ReadOnly: true, UserData: map[string]string{"eed": "true"}})
			if err != nil {
				return err
			}
			fmt.Fprintf(cmd.OutOrStdout(), "Created snapshot #%d (%s)\n", created.Number, created.Description)
			return nil
		},
	}
	cmd.Flags().String("config", "root", "existing Snapper configuration name")
	cmd.Flags().String("description", "Before EED operation", "human-readable snapshot description")
	return cmd
}

func NewVerifyCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "verify",
		Short: "Verify selected EED components without changing system state",
		RunE: func(cmd *cobra.Command, args []string) error {
			component, err := cmd.Flags().GetString("component")
			if err != nil {
				return fmt.Errorf("read component flag: %w", err)
			}
			jsonOutput, err := cmd.Flags().GetBool("json")
			if err != nil {
				return fmt.Errorf("read json flag: %w", err)
			}
			report, err := runVerification(cmd.Context(), component)
			if err != nil {
				return err
			}
			return writeVerificationReport(cmd, report, jsonOutput)
		},
	}
	cmd.Flags().Bool("json", false, "output verification report as JSON")
	cmd.Flags().String("component", "", "verify only the selected component ID")
	return cmd
}

func NewDoctorCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Run read-only system and component diagnostics",
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonOutput, err := cmd.Flags().GetBool("json")
			if err != nil {
				return fmt.Errorf("read json flag: %w", err)
			}
			report, err := runVerification(cmd.Context(), "")
			if err != nil {
				return err
			}
			return writeVerificationReport(cmd, report, jsonOutput)
		},
	}
	cmd.Flags().Bool("json", false, "output doctor report as JSON")
	return cmd
}

func runVerification(ctx context.Context, component string) (verification.Report, error) {
	cfgPath, err := eedconfig.ResolveConfigPath()
	if err != nil {
		return verification.Report{}, fmt.Errorf("resolve config path: %w", err)
	}
	cfg, err := eedconfig.LoadConfig(cfgPath)
	if err != nil {
		return verification.Report{}, fmt.Errorf("load configuration: %w", err)
	}
	detectionReport, err := detection.Detect(ctx)
	if err != nil {
		return verification.Report{}, fmt.Errorf("detect host state: %w", err)
	}
	registry, err := checks.DefaultRegistry()
	if err != nil {
		return verification.Report{}, fmt.Errorf("register verification checks: %w", err)
	}
	engine := verification.Engine{Registry: registry}
	return engine.Run(ctx, verification.Context{Detection: detectionReport, Config: cfg}, component)
}

func writeVerificationReport(cmd *cobra.Command, report verification.Report, jsonOutput bool) error {
	var err error
	if jsonOutput {
		encoder := json.NewEncoder(cmd.OutOrStdout())
		encoder.SetIndent("", "  ")
		err = encoder.Encode(report)
	} else {
		fmt.Fprintf(cmd.OutOrStdout(), "Verification: %s\n", report.OverallStatus)
		fmt.Fprintf(cmd.OutOrStdout(), "Passed: %d  Warnings: %d  Failed: %d  Unknown: %d  Skipped: %d\n",
			report.Summary.Passed, report.Summary.Warnings, report.Summary.Failed, report.Summary.Unknown, report.Summary.Skipped)
		for _, component := range report.Components {
			fmt.Fprintf(cmd.OutOrStdout(), "\n%s: %s\n", component.ComponentID, component.Status)
			for _, check := range component.Checks {
				fmt.Fprintf(cmd.OutOrStdout(), "  [%s] %s", check.Status, check.Name)
				if check.Message != "" {
					fmt.Fprintf(cmd.OutOrStdout(), " — %s", check.Message)
				}
				fmt.Fprintln(cmd.OutOrStdout())
			}
		}
		if report.RecoveryAvailability != "" {
			fmt.Fprintf(cmd.OutOrStdout(), "\nRecovery: %s\n", report.RecoveryAvailability)
		}
	}
	if err != nil {
		return err
	}
	if report.Summary.Failed > 0 {
		return fmt.Errorf("verification reported %d failed checks", report.Summary.Failed)
	}
	return nil
}

func NewInstallCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "install",
		Short: "Execute an approved EED installation or build a read-only preview",
		RunE: func(cmd *cobra.Command, args []string) error {
			dryRun, err := cmd.Flags().GetBool("dry-run")
			if err != nil {
				return fmt.Errorf("read dry-run flag: %w", err)
			}

			jsonOutput, err := cmd.Flags().GetBool("json")
			if err != nil {
				return fmt.Errorf("read json flag: %w", err)
			}

			confirm, _ := cmd.Flags().GetBool("confirm")
			allowUnprotected, _ := cmd.Flags().GetBool("allow-unprotected")

			configPath, err := eedconfig.ResolveConfigPath()
			if err != nil {
				return fmt.Errorf("resolve config path: %w", err)
			}
			deps := defaultPreviewDependencies()
			deps.ConfigPath = configPath
			dataDir, err := readDataDirFlag(cmd)
			if err != nil {
				return err
			}
			if dataDir != "" {
				deps.ManifestPath = dataDir
			}

			if dryRun {
				preview, err := orchestrator.BuildPreview(cmd.Context(), deps)
				if err != nil {
					return fmt.Errorf("build installation preview: %w", err)
				}
				if jsonOutput {
					encoder := json.NewEncoder(cmd.OutOrStdout())
					encoder.SetIndent("", "  ")
					return encoder.Encode(preview)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "Installation preview (read-only)\nProfile: %s\nHost: %s\nStatus: %s\n",
					preview.Profile, preview.Plan.HostOS, preview.Plan.Status)
				fmt.Fprintln(cmd.OutOrStdout(), "\nSelected components:")
				if len(preview.Selected) == 0 {
					fmt.Fprintln(cmd.OutOrStdout(), "  (none)")
				}
				for _, component := range preview.Selected {
					fmt.Fprintf(cmd.OutOrStdout(), "  - %s\n", component)
				}
				fmt.Fprintln(cmd.OutOrStdout(), "\nPackage candidates and installed states:")
				if len(preview.Changes.PackageCandidates) == 0 {
					fmt.Fprintln(cmd.OutOrStdout(), "  (none)")
				}
				packageStates := make(map[string]string, len(preview.PackageReport.Candidates))
				for _, candidate := range preview.PackageReport.Candidates {
					packageStates[candidate.Name] = candidate.State + " (repository=" + candidate.RepositoryStatus + ")"
				}
				for _, pkg := range preview.Changes.PackageCandidates {
					state, ok := packageStates[pkg]
					if !ok {
						state = "unknown"
					}
					fmt.Fprintf(cmd.OutOrStdout(), "  - %s: %s\n", pkg, state)
				}
				fmt.Fprintf(cmd.OutOrStdout(), "\nPackage sizes: total footprint=%d bytes, additional download=%d bytes, additional installed=%d bytes (%s)\n",
					preview.Assessment.Resources.Storage.TotalPackageFootprintBytes,
					preview.Assessment.Resources.Storage.AdditionalDownloadBytes,
					preview.Assessment.Resources.Storage.AdditionalInstallBytes,
					preview.Assessment.Resources.Storage.EstimateConfidence)
				if preview.PortalPlan != nil {
					fmt.Fprintf(cmd.OutOrStdout(), "Portal proposal: desktop=%s, status=%s, path=%s\n",
						preview.PortalPlan.Desktop, preview.PortalPlan.Status, preview.PortalPlan.ConfigPath)
					fmt.Fprintf(cmd.OutOrStdout(), "  Package inventory: %s\n", preview.PortalPlan.PackageInventoryStatus)
					if preview.PortalPlan.PreserveExisting {
						fmt.Fprintf(cmd.OutOrStdout(), "  Existing user config preserved: %s\n", strings.Join(preview.PortalPlan.ExistingConfigPaths, ", "))
					} else {
						fmt.Fprintf(cmd.OutOrStdout(), "  Proposed content:\n%s", preview.PortalPlan.Content)
					}
					if len(preview.PortalPlan.MissingPackages) > 0 {
						fmt.Fprintf(cmd.OutOrStdout(), "  Missing portal packages: %s\n", strings.Join(preview.PortalPlan.MissingPackages, ", "))
					}
				}
				fmt.Fprintf(cmd.OutOrStdout(), "\nSnapshot: %s (%s)\n", preview.Snapshot.Status, preview.Snapshot.Reason)
				fmt.Fprintln(cmd.OutOrStdout(), "\nWarnings:")
				if len(preview.Warnings) == 0 {
					fmt.Fprintln(cmd.OutOrStdout(), "  (none)")
				}
				for _, warning := range preview.Warnings {
					fmt.Fprintf(cmd.OutOrStdout(), "  - %s\n", warning)
				}
				fmt.Fprintln(cmd.OutOrStdout(), "\nExecution blockers:")
				if len(preview.Blockers) == 0 {
					fmt.Fprintln(cmd.OutOrStdout(), "  (none)")
				}
				for _, blocker := range preview.Blockers {
					fmt.Fprintf(cmd.OutOrStdout(), "  - %s\n", blocker)
				}
				return nil
			}

			// Actual installation workflow execution
			opts := orchestrator.InstallOptions{
				DryRun:           false,
				Confirmed:        confirm,
				AllowUnprotected: allowUnprotected,
				UseSudo:          os.Geteuid() != 0,
				Input:            cmd.InOrStdin(),
				Output:           cmd.OutOrStdout(),
			}

			record, err := orchestrator.ExecuteInstallation(cmd.Context(), deps, opts)
			if err != nil {
				if jsonOutput {
					encoder := json.NewEncoder(cmd.OutOrStdout())
					encoder.SetIndent("", "  ")
					_ = encoder.Encode(record)
				}
				if record.PacmanTransaction != nil && record.PacmanTransaction.Status != "" {
					fmt.Fprintf(cmd.ErrOrStderr(), "Pacman transaction: status=%s, exit_code=%d, verification=%s\n",
						record.PacmanTransaction.Status,
						record.PacmanTransaction.ExitCode,
						record.PacmanTransaction.VerificationStatus)
				}
				return err
			}

			if jsonOutput {
				encoder := json.NewEncoder(cmd.OutOrStdout())
				encoder.SetIndent("", "  ")
				return encoder.Encode(record)
			}

			fmt.Fprintf(cmd.OutOrStdout(), "Installation completed: status=%s\n", record.OverallStatus)
			if record.SnapshotID > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "Snapshot created: #%d (%s)\n", record.SnapshotID, record.SnapshotDescription)
			}
			if record.PacmanTransaction != nil {
				if record.PacmanTransaction.Status != "" {
					fmt.Fprintf(cmd.OutOrStdout(), "Pacman transaction: status=%s, exit_code=%d, verification=%s\n",
						record.PacmanTransaction.Status,
						record.PacmanTransaction.ExitCode,
						record.PacmanTransaction.VerificationStatus)
				}
				if len(record.PacmanTransaction.InstalledByEED) > 0 {
					fmt.Fprintf(cmd.OutOrStdout(), "Packages installed: %s\n", strings.Join(record.PacmanTransaction.InstalledByEED, ", "))
				}
			}
			if record.ConfigurationReport != nil && len(record.ConfigurationReport.Deployed) > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "Configuration files deployed: %d\n", len(record.ConfigurationReport.Deployed))
			}
			if record.ServiceResult != nil && len(record.ServiceResult.Applied) > 0 {
				fmt.Fprintf(cmd.OutOrStdout(), "Services configured: %d\n", len(record.ServiceResult.Applied))
			}
			if record.VerificationReport != nil {
				fmt.Fprintf(cmd.OutOrStdout(), "Verification: %s (passed: %d, warnings: %d, failed: %d)\n",
					record.VerificationReport.OverallStatus,
					record.VerificationReport.Summary.Passed,
					record.VerificationReport.Summary.Warnings,
					record.VerificationReport.Summary.Failed)
			}
			return nil
		},
	}
	cmd.Flags().Bool("dry-run", false, "build a read-only installation preview; does not modify the system")
	cmd.Flags().Bool("json", false, "output the preview or installation record as JSON")
	cmd.Flags().Bool("confirm", false, "automatically confirm the installation transaction")
	cmd.Flags().Bool("allow-unprotected", false, "allow proceeding when snapshot protection is unavailable")
	return cmd
}

func readDataDirFlag(cmd *cobra.Command) (string, error) {
	flag := cmd.Flag("data-dir")
	if flag == nil {
		return "", nil
	}
	return flag.Value.String(), nil
}

func NewTestDriveCmd() *cobra.Command {
	return NewTestDriveCmdWithDependencies(defaultPreviewDependencies())
}

func defaultPreviewDependencies() orchestrator.Dependencies {
	deps := orchestrator.DefaultDependencies()
	archInspector := archpacman.NewInspector()
	aptInspector := debianapt.NewInspector()
	deps.InspectPackages = archInspector.Inspect
	deps.InspectPackagesForOS = func(ctx context.Context, osID string, packages []string) (packageinfo.Report, error) {
		switch strings.ToLower(strings.TrimSpace(osID)) {
		case "arch", "arch linux":
			return archInspector.Inspect(ctx, packages)
		case "ubuntu", "debian":
			return aptInspector.Inspect(ctx, packages)
		default:
			return packageinfo.UnavailableReport(packages, "not_applicable_host", "no native read-only package inspector is available for this distribution"), nil
		}
	}
	deps.PlanPortals = archportals.BuildHostPlan
	return deps
}

func NewTestDriveCmdWithDependencies(deps orchestrator.Dependencies) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "test-drive",
		Short: "Run a read-only installation test drive",
		RunE: func(cmd *cobra.Command, args []string) error {
			jsonOutput, err := cmd.Flags().GetBool("json")
			if err != nil {
				return fmt.Errorf("read json flag: %w", err)
			}
			if deps.ConfigPath == "" {
				configPath, err := eedconfig.ResolveConfigPath()
				if err != nil {
					return fmt.Errorf("resolve config path: %w", err)
				}
				deps.ConfigPath = configPath
			}
			dataDir, err := readDataDirFlag(cmd)
			if err != nil {
				return err
			}
			if dataDir != "" {
				deps.ManifestPath = dataDir
			}
			if jsonOutput {
				report, err := testdrive.Build(cmd.Context(), deps)
				if err != nil {
					return err
				}
				return testdrive.WriteJSON(cmd.OutOrStdout(), report)
			}
			_, err = testdrive.RunInteractive(cmd.Context(), deps, cmd.InOrStdin(), cmd.OutOrStdout())
			return err
		},
	}
	cmd.Flags().Bool("json", false, "output a non-interactive read-only test-drive report as JSON")
	return cmd
}

func NewConfigCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "View and validate the EED configuration",
	}
	cmd.AddCommand(NewConfigShowCmd())
	cmd.AddCommand(NewConfigValidateCmd())
	return cmd
}

func NewConfigShowCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Display the active EED configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := eedconfig.ResolveConfigPath()
			if err != nil {
				return err
			}

			cfg, err := eedconfig.LoadConfig(path)
			if err != nil {
				return err
			}

			data, err := toml.Marshal(cfg)
			if err != nil {
				return fmt.Errorf("marshal config for display: %w", err)
			}
			_, err = fmt.Fprint(cmd.OutOrStdout(), string(data))
			return err
		},
	}
	return cmd
}

func NewConfigValidateCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate the active EED configuration",
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := eedconfig.ResolveConfigPath()
			if err != nil {
				return err
			}

			cfg, err := eedconfig.LoadConfig(path)
			if err != nil {
				return err
			}

			if err := eedconfig.Validate(cfg); err != nil {
				return fmt.Errorf("validation failed: %w", err)
			}
			fmt.Fprintln(cmd.OutOrStdout(), "Configuration is valid.")
			return nil
		},
	}
	return cmd
}
