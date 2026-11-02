# EED — Implement the Real Arch Linux Installation Engine

You are working on EED (Eliminate Endless Torture), a modular Linux workstation configuration platform written primarily in Go.

Your task is to modify the existing source code and replace incomplete installation placeholders with functional, secure implementations.

The development environment is Ubuntu 24.04, but the primary installation backend being implemented is Arch Linux.

IMPORTANT:
Ubuntu is the development environment, NOT the target system for Arch installation.

All development, compilation and unit testing must be safe on Ubuntu.

Actual Pacman installation tests must only be performed inside a disposable Arch Linux virtual machine.

Do not execute real package installations or modify the Ubuntu host.

# PHASE 1 — COMPLETE SOURCE CODE AUDIT

Before modifying anything:

1. Inspect the complete repository.
2. Read go.mod and existing project architecture.
3. Inspect all package manager interfaces and implementations.
4. Inspect the installation orchestrator.
5. Inspect configuration deployment code.
6. Inspect service management code.
7. Inspect component manifests and dependency resolution.
8. Inspect resource discovery and manifest loading.
9. Identify all Execution Blockers and not implemented placeholders.

Create a report mapping each blocker to its source file, function and required implementation.

Preserve working functionality. Do not rewrite the project unnecessarily.

# PHASE 2 — IMPLEMENT PACMAN TRANSACTION ENGINE

Replace the Pacman installation placeholder with a real Arch Linux package transaction engine.

Requirements:

- Use the existing package manager interfaces.
- Use Go exec.CommandContext.
- Pass command arguments separately.
- Never construct shell commands from concatenated user-controlled strings.
- Preserve the native Pacman dependency resolver.
- Implement installed-package inventory.
- Implement package availability checks.
- Implement transaction result reporting.
- Track EED installation ownership.

## Arch Linux safety

Arch Linux does not support partial system upgrades.

Never execute:

pacman -Sy package
pacman -Sy followed by pacman -S package

For installation transactions requiring repository synchronization, use the full-system-upgrade strategy:

pacman -Syu --needed package1 package2

Requirements:

1. Show the user that a full system upgrade may occur.
2. Present the proposed transaction before execution.
3. Obtain explicit confirmation.
4. Do not use --noconfirm by default.
5. Preserve Pacman's native confirmation and conflict handling.
6. Handle cancellation and nonzero exit codes.
7. Never automatically remove unrelated packages.
8. Do not run EED itself as root.
9. Request elevated privileges only for the approved package transaction.
10. Verify installation afterward using Pacman queries.

The installer must explicitly reject unsupported host/target combinations.

For example, an Arch installation transaction must never execute on Ubuntu.

Do not invoke APT as a replacement for Pacman.

## Transaction ownership

Track:
- Transaction ID.
- Timestamp.
- Selected component IDs.
- Explicit package targets.
- Packages installed before the transaction.
- Packages installed afterward, where determinable.
- Transaction result.
- Errors and warnings.

Do not claim that every transitive dependency is exclusively owned by EED.

Never automatically remove packages during recovery unless ownership and safety are established.

# PHASE 3 — IMPLEMENT CONFIGURATION MANAGER

Replace the configuration deployment placeholder.

Implement native Go filesystem operations.

Support:

- File deployment.
- Directory deployment.
- Safe symlink creation.
- Configuration templates.
- Component-specific destinations.
- User-level configuration under $HOME/.config.

## Safety requirements

Before deploying any file:

1. Inspect the destination using Lstat.
2. Determine whether it already exists.
3. Determine whether it is a regular file, directory or symlink.
4. Check whether EED previously created or owns it.
5. Preserve user modifications.
6. Create a backup when replacing an existing unmanaged file.
7. Ask for explicit approval before replacing user configuration.

Do not blindly execute ln -sf.

Use atomic deployment where possible.

Prevent:
- Path traversal.
- Symlink-based path escape.
- Overwriting files outside approved destinations.
- Executing arbitrary shell commands from component manifests.

Create an EED configuration ownership journal.

Track source, destination, backup, component ID and deployment timestamp.

Implement safe rollback.

Never silently delete user-modified configuration files.

# PHASE 4 — IMPLEMENT SYSTEMD SERVICE MANAGER

Replace the service manager placeholder.

Support:

- Service discovery.
- Unit existence checks.
- Active state inspection.
- Enabled state inspection.
- System services.
- User services.
- Enable operations.
- Start operations.
- Enable-and-start operations.

Use Go process execution APIs.

System services:

systemctl --system

User services:

systemctl --user

Requirements:

1. Detect whether systemd is actually available.
2. Never assume every Linux distribution uses systemd.
3. Distinguish system and user services.
4. Verify that requested unit files exist.
5. Do not enable every installed service automatically.
6. Respect component-specific service requirements.
7. Show proposed changes before execution.
8. Require user approval.
9. Track changes for recovery.
10. Never disable unrelated services.

Do not use systemctl mutations during Test Drive or dry-run.

# PHASE 5 — FIX SHARED LIBRARY SONAME DEPENDENCY RESOLUTION

## Current problem

The dependency resolver attempts to query library names as ordinary packages using pacman -Si.

Examples:

libssl.so
libcurl.so
libnghttp2.so

These are shared-library SONAME dependencies and must not be treated as ordinary package names.

IMPORTANT:

Do not simply skip dependencies containing ".so".

Do not silently ignore virtual dependencies such as opengl-driver.

Implement proper dependency classification and provider resolution.

Support:

- Ordinary package dependencies.
- Versioned package dependencies.
- Shared-library SONAME dependencies.
- Versioned SONAME requirements.
- Virtual package dependencies.
- Providers declared through Provides.
- Optional dependencies.
- Missing providers.
- Incomplete repository metadata.

Investigate libalpm integration and use an authoritative Pacman-compatible dependency resolver wherever practical.

For SONAME requirements:

1. Preserve the library name.
2. Preserve ABI version requirements.
3. Inspect repository metadata and provided SONAMEs.
4. Find compatible package providers.
5. Respect version constraints.
6. Distinguish unresolved dependencies from unavailable metadata.

For virtual dependencies:

- Inspect provider declarations.
- Identify compatible providers.
- Do not query virtual dependency names as if they were ordinary package names.

Test with:
- libssl.so
- libcurl.so
- libnghttp2.so
- Versioned SONAME dependencies.
- opengl-driver.
- Packages with Provides.
- Missing providers.

Never report successful dependency resolution when a required provider remains unresolved.

Pacman must remain responsible for final transaction dependency resolution.

# PHASE 6 — FIX RESOURCE AND MANIFEST DISCOVERY

Current problem:

EED sometimes reports:

components/manifests not found

The application must not depend on the current working directory.

Implement resource discovery with the following priority:

1. Explicit --data-dir option.
2. EED_DATA_DIR environment variable.
3. Embedded built-in resources using Go embed.
4. System resource directory: /usr/share/eed.
5. Development repository discovery.

For development mode, locate the repository root using markers such as:
- go.mod
- components/manifests

Do not assume the executable always resides inside dist/.

Resolve executable symlinks where appropriate.

Support launching EED from:
- Repository root.
- dist/.
- Another working directory.
- A system-installed location.

Return actionable errors if resources cannot be found.

Do not create empty manifest directories automatically.

# PHASE 7 — TEST DRIVE AND DRY-RUN SAFETY

Both commands must remain completely read-only:

eed test-drive
eed install --dry-run

They must never:

- Install packages.
- Synchronize Pacman databases.
- Modify configuration files.
- Create symlinks.
- Enable or start services.
- Create filesystem snapshots.
- Modify package databases.
- Invoke sudo or pkexec.

They must use the same planning and dependency-resolution logic as actual installation.

The report must distinguish:

- Planned operations.
- Executed operations.
- Unavailable functionality.
- Unknown information.
- Confirmed compatibility.
- Unresolved dependencies.

Never claim an operation succeeded if it was only simulated.

# PHASE 8 — VERIFICATION AND RECOVERY INTEGRATION

Connect the installation managers to the existing verification and recovery architecture.

After installation:

- Verify explicitly requested packages.
- Verify deployed configuration files.
- Verify symlinks and their targets.
- Verify requested services.
- Write transaction and ownership records.
- Report incomplete operations.
- Support safe restoration of previous configuration.

Do not claim full rollback protection solely because Snapper is installed.

Test actual rollback separately in the Arch VM.

# PHASE 9 — REGRESSION TESTING

Add unit and integration tests using mocked command execution.

Cover:

## Package manager
- Safe full-upgrade arguments.
- No default --noconfirm.
- User cancellation.
- Pacman nonzero exit.
- Unsupported host rejection.
- Installed package inventory.
- Ownership tracking.

## Dependency resolver
- Ordinary package dependencies.
- SONAME resolution.
- Versioned SONAME.
- Virtual providers.
- Missing dependencies.
- Incomplete metadata.

## Configuration manager
- New file deployment.
- Existing file backup.
- Existing symlink handling.
- User modification preservation.
- Path traversal prevention.
- Rollback.

## Service manager
- System services.
- User services.
- Missing units.
- Active and enabled states.
- Failed operations.

## Resource discovery
- Root directory launch.
- dist directory launch.
- Different working directory.
- Explicit data directory.
- Missing resources.

## Safety
- Test Drive read-only behavior.
- Dry-run read-only behavior.
- No unintended privileged commands.
- No Ubuntu package installation.

# PHASE 10 — BUILD AND DELIVERY

After implementation:

Run:

gofmt -w .
go test ./...
go vet ./...
go test -race ./...

Build:

go build -o dist/eed ./cmd/eed

Inspect:

git diff --check
git diff --stat

Do not execute actual package transactions on the Ubuntu development machine.

Prepare detailed instructions for testing the resulting binary in a disposable Arch Linux VM.

# FINAL DELIVERABLES

Provide:

1. Architecture audit.
2. Modified source files.
3. Implemented Pacman transaction engine.
4. Implemented Configuration Manager.
5. Implemented Service Manager.
6. Corrected SONAME dependency resolver.
7. Improved resource discovery.
8. Verification and ownership journal integration.
9. Unit and integration test results.
10. Remaining blockers.
11. Arch VM testing instructions.

IMPORTANT FINAL RULES:

Do not remove Execution Blockers merely to make the report look successful.

Remove each blocker only after its implementation exists and corresponding tests pass.

Do not weaken compatibility or hardware checks to force installation.

Do not claim EED is production-ready until the entire workflow has been tested on a disposable Arch Linux VM, including installation, verification and recovery.