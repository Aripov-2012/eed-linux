# EED DEVELOPMENT TASK
# FINAL MILESTONE — INTEGRATION, TESTING & STABLE RELEASE
## EED 0.9.0 → EED 1.0.0

You are developing EED (Eliminate Endless Torture), an open-source modular Arch Linux configuration and automation platform written primarily in Go.

This is the final major development milestone before the first stable release.

Your responsibility is to integrate the existing subsystems into one reliable application, perform comprehensive testing, resolve architectural inconsistencies and prepare the EED 1.0.0 release.

IMPORTANT: Do not assume that previous milestones are complete merely because their planned version numbers exist.

First inspect the actual repository and determine what is implemented, tested, incomplete or missing.

Do not fabricate functionality or replace incomplete implementations with fake success responses.

Preserve existing work and improve it incrementally.

# PART A — PROJECT AUDIT

## 1. Inspect the complete project

Inspect:

- Go source code.
- CLI commands.
- Configuration schemas.
- Component manifests.
- Detection Engine.
- Customization Engine.
- Installation Planner.
- Compatibility Engine.
- Resource Analyzer.
- Storage Analyzer.
- Snapshot Engine.
- Package Manager Engine.
- Configuration Engine.
- Service Manager.
- Verification Engine.
- Recovery Manager.
- Existing tests.
- Build scripts.
- Documentation.
- CI configuration.

Create an audit report:

`docs/RELEASE_AUDIT.md`

For every subsystem, classify its state as:

- IMPLEMENTED_AND_TESTED
- IMPLEMENTED_BUT_UNTESTED
- PARTIALLY_IMPLEMENTED
- MISSING
- BLOCKED_BY_EXTERNAL_REQUIREMENTS

Identify:

- Duplicated functionality.
- Inconsistent Go interfaces.
- Incompatible data structures.
- Inconsistent JSON schemas.
- Incorrect CLI behavior.
- Missing error handling.
- Security concerns.
- Incomplete tests.
- Unverified rollback assumptions.

Do not mark an untested subsystem as fully working.

Do not delete or replace existing files just to make the architecture look cleaner.

# PART B — UNIFIED INSTALLATION ORCHESTRATOR

## 2. Create the central orchestrator

Implement a central installation orchestrator that coordinates all existing modules.

Suggested structure:

```text
internal/
└── orchestrator/
    ├── orchestrator.go
    ├── workflow.go
    ├── stages.go
    ├── execution.go
    ├── context.go
    ├── journal.go
    ├── progress.go
    ├── cancellation.go
    ├── errors.go
    └── orchestrator_test.go
```

Adapt the structure to the existing architecture.

The orchestrator must not reimplement the internal logic of other subsystems.

It must use their public internal APIs.

Responsibilities:

- Coordinate installation stages.
- Validate dependencies between stages.
- Manage execution state.
- Manage user confirmation.
- Track progress.
- Handle errors.
- Handle cancellation.
- Save execution journals.
- Prevent unsafe concurrent installation operations.
- Produce a final installation report.

Avoid circular dependencies between modules.

Define clear interfaces for orchestration boundaries.

Do not create excessive abstractions without a concrete need.

## 3. Installation workflow

Implement this workflow using existing subsystems:

```text
START
  |
  v
LOAD USER CONFIGURATION
  |
  v
DETECT SYSTEM
  |
  v
LOAD COMPONENT REGISTRY
  |
  v
CREATE INSTALLATION PLAN
  |
  v
CHECK COMPATIBILITY
  |
  v
ANALYZE RESOURCES
  |
  v
ANALYZE STORAGE
  |
  v
CHECK SNAPSHOT CAPABILITY
  |
  v
BUILD COMPLETE INSTALLATION PREVIEW
  |
  v
USER CONFIRMATION
  |
  v
CREATE EED BASELINE / SNAPSHOT
  |
  v
INSTALL PACKAGES
  |
  v
APPLY CONFIGURATIONS
  |
  v
CONFIGURE SERVICES
  |
  v
VERIFY COMPONENTS
  |
  v
PERSIST FINAL STATE
  |
  v
GENERATE INSTALLATION REPORT
```

Not all stages must execute for every installation.

For example:
- A user selecting no graphical environment must not enter desktop-specific stages.
- An optional component may be skipped.
- An existing application does not necessarily need installation.
- An existing desktop environment may be preserved.
- An unavailable optional service must not automatically block unrelated components.

Stage execution must be determined by the actual Installation Plan.

## 4. Preconditions and blocking conditions

Before any system modification:

- Confirm that the current OS is supported for installation.
- Confirm architecture support.
- Check required privileges.
- Validate the user configuration.
- Validate component manifests.
- Validate dependency resolution.
- Check package availability.
- Check compatibility.
- Check required resources.
- Analyze storage.
- Evaluate snapshot capability.
- Confirm that no package manager operation is already running.

Unsupported operating systems must remain usable for diagnostics and development, but installation must be blocked outside the supported Arch target.

Unknown information must be reported honestly.

Do not convert UNKNOWN into PASS.

## 5. Installation preview

Implement:

```bash
eed install --dry-run
```

The preview must show:

- User-selected profile.
- Selected desktop environment.
- Selected components.
- Additional dependencies.
- Packages already installed.
- Packages to install.
- Expected system update.
- Configuration files that will be created or changed.
- Services that will be modified.
- Storage information.
- Snapshot availability.
- Warnings.
- Blocking conditions.

The preview must be read-only.

Do not perform package queries or storage operations that modify databases or system state without clear justification.

Do not claim that an estimate is exact when package or storage metadata is incomplete.

## 6. User confirmation

Before changing the system, display a complete summary.

The user must understand:

- What packages will be installed.
- That Arch Linux will undergo a full system upgrade.
- Which configuration files will change.
- Which services will be modified.
- Whether a system Snapshot will be created.
- Whether full rollback is actually supported.
- What happens if Snapshot protection is unavailable.

Require explicit confirmation.

If the user continues without Snapshot protection, require an additional explicit acknowledgement.

An explicit option such as:

`--allow-unprotected`

may be implemented for non-interactive operation, but it must not silently bypass safety requirements.

Do not use automatic confirmation for the underlying pacman transaction.

Preserve pacman's own transaction confirmation.

## 7. Installation execution

Execute stages using the existing subsystem APIs.

Do not invoke EED's own commands as child processes.

Do not start nested instances of the EED CLI.

Do not duplicate implementation logic in the orchestrator.

Use direct Go function calls and typed interfaces.

Every stage must report:

- Stage ID.
- Started time.
- Completed time.
- Result.
- Errors.
- Warnings.
- Changes made.
- Related operation IDs.

Suggested stage states:

- pending
- running
- completed
- skipped
- failed
- cancelled
- interrupted
- requires_manual_action

## 8. Failure handling

A failure must stop dependent stages.

Examples:

If package installation fails:
- Do not continue with configuration deployment.
- Collect the actual package inventory.
- Save the transaction result.
- Preserve pacman error output.

If configuration deployment fails:
- Preserve created backups.
- Do not silently overwrite conflicting files.
- Stop dependent operations.
- Produce a Recovery Plan where applicable.

If a required service fails:
- Record the issue.
- Determine whether it blocks the requested configuration.
- Do not report overall success incorrectly.

If verification reports warnings:
- Preserve successful components.
- Show the warnings clearly.

If a component fails:
- Do not automatically perform a full-system rollback without a separately validated rollback plan and user confirmation.

Never assume that a partially completed operation made no changes.

## 9. Cancellation and interruption

Support graceful cancellation through context cancellation and operating system signals.

When the user cancels:

- Stop scheduling new stages.
- Allow an active package transaction to terminate safely according to its existing process-management contract.
- Do not kill pacman in an unsafe way.
- Record the current workflow state.
- Inspect current system state where possible.
- Preserve recovery metadata.
- Produce an interruption report.

Do not automatically resume an interrupted installation.

A future resume attempt must re-run detection and planning against the actual current system.

Already completed operations must not be blindly repeated.

## 10. Idempotency

An installation workflow must be safe to repeat.

On a second run:

- Re-detect installed packages.
- Recheck configurations.
- Recheck service states.
- Identify completed operations.
- Generate a new plan based on actual state.
- Avoid unnecessary modifications.

Do not assume that the previous installation journal accurately describes current system state.

The real system remains the authority.

# PART C — INSTALLATION JOURNAL AND STATE

## 11. Unified installation journal

Implement a versioned journal.

Suggested location:

`/var/lib/eed/installations/`

Store:

- Operation ID.
- User profile.
- EED version.
- Target OS.
- Architecture.
- Selected components.
- Installation Plan reference.
- Assessment reference.
- Snapshot ID.
- Package transaction IDs.
- Configuration operation IDs.
- Service operation IDs.
- Verification report.
- Current stage.
- Overall status.
- Started and completed timestamps.
- Errors and warnings.

Use atomic persistence.

Use appropriate system-level permissions.

Do not store credentials, passwords or authentication secrets.

Use stable IDs to connect the installation journal with existing package, configuration, service and snapshot manifests.

## 12. State consistency

Before trusting stored state:

- Validate schema versions.
- Validate file integrity where applicable.
- Detect missing records.
- Detect incomplete operations.
- Compare important recorded state with actual system state.

Do not silently migrate unknown schema versions.

Provide explicit migration errors or supported migration functions.

Do not delete previous journals automatically.

# PART D — CLI INTEGRATION

## 13. Final command structure

Ensure the following commands exist and behave consistently:

```bash
eed
eed help
eed version

eed wizard
eed config show
eed config validate
eed config plan
eed config apply
eed config status
eed config diff

eed detect
eed assess
eed plan

eed install
eed install --dry-run
eed install --json

eed storage analyze
eed snapshot status
eed snapshot list
eed snapshot create

eed packages status
eed packages list
eed packages inspect
eed packages preview
eed packages install
eed packages transactions

eed services status
eed services plan
eed services apply

eed verify
eed doctor

eed recovery status
eed recovery list
eed recovery plan
eed recovery apply

eed uninstall --dry-run
eed uninstall

eed rollback
```

Only expose commands whose underlying implementation is actually functional.

Do not register fake commands with placeholder responses.

For each command:

- Provide useful help.
- Use consistent flags.
- Return appropriate error codes.
- Support structured JSON output where appropriate.
- Keep JSON stdout free from decorative output.
- Send diagnostics to stderr.
- Clearly distinguish read-only and modifying commands.

Add a command that displays overall EED installation history and status if it can reuse the existing journal.

# PART E — END-TO-END TESTING

## 14. Unit testing

Run and improve all existing unit tests.

Required:

```bash
gofmt -w .
go test ./...
go vet ./...
go test -race ./...
go build ./cmd/eed
```

Ensure tests do not depend on the developer's actual installed programs or hardware.

Use mocks, fixtures and dependency injection.

Do not make ordinary tests invoke real pacman transactions, systemctl modifications or snapshot rollback.

## 15. Integration testing

Build integration tests for subsystem boundaries.

Test these data flows:

1. Wizard → Configuration.
2. Configuration → Customization Engine.
3. Detection → Installation Planner.
4. Component Registry → Dependency Resolver.
5. Installation Plan → Compatibility Engine.
6. Installation Plan → Resource Analyzer.
7. Detection → Storage Analyzer.
8. Storage → Snapshot Capability.
9. Installation Plan → Package Manager.
10. Package Manager → Package Tracking.
11. Component Manifest → Configuration Engine.
12. Component Manifest → Service Manager.
13. Installation → Verification.
14. Verification → Doctor.
15. Tracking Manifests → Recovery Manager.

Test error propagation at every boundary.

## 16. End-to-end scenarios

Prepare automated scenarios using fake or mocked system backends.

At minimum:

### Scenario A: Developer workstation

User selects:

- Development.
- KDE Plasma.
- Git.
- Docker.
- Python.
- Node.js.
- VS Code.
- Catppuccin.

Expected:
- Correct plan.
- Dependencies resolved.
- No conflicts.
- Resource assessment generated.
- Dry-run generated.
- No unexpected system modification.

### Scenario B: Minimal system

User selects:

- Minimal system.
- No GUI.
- No desktop environment.
- Git.
- Basic terminal tools.

Expected:
- No graphical packages selected.
- No display manager configured.
- Valid installation plan.

### Scenario C: Existing desktop

User chooses to preserve GNOME.

Expected:
- GNOME is not removed.
- Existing display manager is not replaced automatically.
- Unselected desktop components are not installed.

### Scenario D: Insufficient storage

Expected:
- Installation blocked or warning policy applied according to explicit configuration.
- Snapshot requirements explained.
- No package transaction started without required approval.

### Scenario E: Snapshot unavailable

Expected:
- User sees the limitation.
- Installation cannot silently continue.
- Explicit unprotected-install acknowledgement is required.

### Scenario F: Package failure

Expected:
- Dependent configuration stages are not executed.
- Current package inventory is recorded.
- Recovery metadata remains available.

### Scenario G: Configuration conflict

Expected:
- Existing user configuration is preserved.
- Installation reports the conflict.
- No silent overwrite occurs.

### Scenario H: Service failure

Expected:
- Service failure is recorded.
- Verification identifies the actual failed state.
- Overall report is not falsely successful.

### Scenario I: Interrupted installation

Expected:
- Current stage is recorded.
- Completed operations remain identifiable.
- A later run re-detects the system instead of blindly repeating everything.

### Scenario J: Unsupported operating system

Run under Ubuntu.

Expected:
- Detection works.
- Planning and diagnostic features work where supported.
- Actual Arch installation is blocked.
- No APT or other package manager is used as a substitute for pacman.

# PART F — QEMU SYSTEM TESTING

## 17. Create a manual VM testing strategy

Prepare instructions for separate disposable Arch Linux QEMU virtual machines.

Test environments:

1. Arch Linux + Btrfs + properly configured Snapper.
2. Arch Linux + ext4.
3. Arch Linux + KDE Plasma.
4. Arch Linux + XFCE.
5. Arch Linux + Hyprland.
6. Arch Linux minimal TTY installation.
7. Arch Linux with limited available disk space.
8. Arch Linux with restricted RAM.

Each test must document:

- VM configuration.
- Disk layout.
- Filesystem layout.
- Selected EED profile.
- Expected result.
- Actual result.
- Logs.
- Known limitations.

Do not claim all hardware driver configurations are validated by QEMU.

GPU-specific validation may require physical hardware.

Do not conduct destructive snapshot tests on the developer's daily computer.

Use disposable virtual machines and known-good VM checkpoints.

Create a VM test matrix under:

`docs/testing/VM_TEST_MATRIX.md`

# PART G — SECURITY AND QUALITY AUDIT

## 18. Security review

Perform a systematic audit of:

### Privileges
- Root versus ordinary user operations.
- Narrow privilege escalation.
- User identity preservation.
- Safe user-service management.

### Filesystem
- Path traversal.
- Symlink attacks.
- Atomic file writes.
- File ownership.
- Permission handling.
- Backup preservation.

### Commands
- Argument injection.
- Shell injection.
- Timeouts.
- Process cancellation.
- Standard output and error handling.

### Package management
- Partial upgrade prevention.
- Signature verification.
- Pacman lock handling.
- Package ownership.
- Safe removal planning.

### Recovery
- Backup integrity.
- Snapshot ownership.
- Btrfs subvolume coverage.
- Boot configuration validation.
- Interrupted recovery.

### Configuration
- TOML validation.
- Trusted template boundaries.
- Schema validation.
- User modification preservation.

### State management
- Atomic journals.
- File permissions.
- Concurrent operations.
- Schema versioning.
- Interrupted operation detection.

Produce:

`docs/SECURITY_AUDIT.md`

Fix high-severity findings before declaring the project ready for a stable release.

Do not dismiss security problems merely because EED is open source.

# PART H — PERFORMANCE AND RELIABILITY

## 19. Performance review

Measure:

- Detection Engine runtime.
- Installation Plan generation.
- Compatibility assessment.
- Storage analysis.
- Configuration rendering.
- Verification runtime.
- Memory usage of EED.

Avoid premature optimization.

Prioritize eliminating unnecessary external command execution and duplicated system scans.

Use parallel read-only checks only when they do not create race conditions or overwhelm system resources.

Do not run package transactions, configuration writes or service changes concurrently unless their safety has been explicitly established.

## 20. Reliability review

Test:

- Large component registries.
- Missing optional tools.
- Malformed manifests.
- Unknown configuration schema versions.
- Permission limitations.
- Missing system services.
- Network disconnections.
- Interrupted processes.
- Corrupt tracking state.
- Insufficient storage.
- Partial subsystem availability.

Unknown or unavailable data must remain explicit.

Never fabricate success.

# PART I — EED 1.0.0 RELEASE PREPARATION

## 21. Version management

Implement a centralized version system.

The release should identify:

- Semantic version.
- Git commit.
- Build date where appropriate.
- Build target.
- Release channel.

Example:

```text
EED 1.0.0
Eliminate Endless Torture

Channel: stable
Architecture: x86_64
```

Avoid duplicated hardcoded version strings throughout the project.

Use Go build-time variables for release metadata.

# 22. Build and packaging

Prepare:

- Linux x86_64 binary.
- Linux ARM64 binary only if its architecture support and relevant functionality are verified.
- Development build.
- Release build.
- Version information.
- Build instructions.
- SHA-256 checksums.

Use reproducible build practices where possible.

Build using appropriate flags such as:

- `-trimpath`
- controlled `-ldflags`
- consistent Go toolchain versions

Ensure that runtime dependencies are documented.

Do not claim that the application is fully self-contained if required external system tools remain necessary.

Prepare an Arch Linux packaging recipe such as a PKGBUILD if the current project architecture supports it.

Validate the package in a disposable Arch VM.

Do not publish to AUR automatically.

Do not invent signing keys or claim release signatures without actually signing the artifacts.

# 23. GitHub Actions

Prepare CI workflows for:

- Go formatting checks.
- Unit tests.
- Race tests where practical.
- Go vet.
- Build verification.
- Linting.
- Release builds.

Use appropriate permissions and maintained GitHub Actions.

Use read-only repository permissions by default.

Do not expose secrets in build logs.

Release publication must be a separate explicitly triggered workflow.

Do not automatically publish a public 1.0.0 release during ordinary development.

# 24. Documentation

Review and finalize:

```text
README.md
CHANGELOG.md
CONTRIBUTING.md
SECURITY.md
LICENSE
docs/
```

Documentation must explain:

- What EED is.
- Supported operating systems.
- Supported architectures.
- Requirements.
- Installation instructions.
- Setup Wizard.
- Available components.
- Hardware detection.
- Storage and snapshot requirements.
- Configuration management.
- Recovery capabilities.
- CLI reference.
- Troubleshooting.
- Known limitations.
- Development setup.
- Testing.

Clearly distinguish features that are fully supported from features that are experimental or unavailable.

Do not claim that EED supports a desktop environment or filesystem backend unless it has been tested.

Do not invent a project license. If no license has been chosen, report this as a release blocker and ask the project owner.

# 25. RELEASE ACCEPTANCE CRITERIA

EED 1.0.0 may be considered ready only when:

- The Go application builds successfully.
- Unit tests pass.
- Integration tests pass.
- Critical end-to-end scenarios pass.
- Arch Linux package transactions have been tested in a disposable VM.
- Configuration deployment preserves existing user data.
- Service operations behave correctly.
- Snapshot creation is tested on a supported Btrfs layout.
- Rollback claims match actual tested capability.
- Recovery manifests correctly identify EED-owned changes.
- No high-severity unresolved security findings remain.
- CLI documentation is complete.
- Release artifacts are built and checksummed.
- Supported functionality and limitations are documented.

Do not mark a criterion complete based on source-code existence alone.

A manual QEMU test must be recorded as manually verified, not automatically passed by CI.

# 26. FINAL DIRECTORY EXPECTATIONS

Keep the existing architecture.

Add or update only necessary integration and release files, such as:

```text
cmd/
internal/
components/
profiles/
configs/
scripts/
tests/
docs/
.github/
Makefile
go.mod
go.sum
README.md
CHANGELOG.md
CONTRIBUTING.md
SECURITY.md
LICENSE
```

Do not reorganize the complete repository without a compelling reason.

# 27. FINAL EXECUTION RULES

- Preserve all working functionality.
- Do not write fake implementations.
- Do not silently skip failing modules.
- Do not claim tests passed unless they actually ran.
- Do not perform destructive tests on the developer's host.
- Do not run the entire EED application as root.
- Do not bypass package-manager safeguards.
- Do not automatically reboot.
- Do not automatically publish releases.
- Do not push commits without explicit authorization.
- Do not delete project history.
- Do not hide warnings.
- Do not claim full rollback where it is not verified.

The current development host may be Ubuntu, but actual Arch installation tests must run in disposable Arch Linux VMs.

# 28. FINAL REPORT

After implementation, produce:

1. Repository audit summary.
2. Subsystems found implemented or incomplete.
3. Architecture integration summary.
4. Unified installation workflow.
5. Installation journal structure.
6. Error and cancellation behavior.
7. Security audit results.
8. Test coverage summary.
9. Unit test results.
10. Integration test results.
11. QEMU test instructions and actual results.
12. Build results.
13. Release artifact list.
14. Documentation changes.
15. Known limitations.
16. Remaining release blockers.
17. Recommended release decision: READY FOR RELEASE CANDIDATE or NOT READY.

Do not claim EED 1.0.0 is stable merely because compilation succeeds.

Complete integration and testing first. Prepare the 1.0.0 release artifacts only when the acceptance criteria are satisfied.