# EED

EED (Eliminate Endless Torture) is a modular Arch Linux configuration and automation platform.

This repository is an active development project. It includes read-only host detection, configuration normalization, component metadata and planning, compatibility/resource assessment, storage/snapshot diagnostics, and registry-based verification. Package/configuration/service deployment and recovery are not implemented; this is not a stable 1.0.0 release.

## Quick start

```bash
go run ./cmd/eed --help
go run ./cmd/eed version
go run ./cmd/eed detect
go run ./cmd/eed detect --json
go run ./cmd/eed wizard
go run ./cmd/eed plan
go run ./cmd/eed plan --json
go run ./cmd/eed assess
go run ./cmd/eed assess --json
go run ./cmd/eed storage analyze
go run ./cmd/eed snapshot status
go run ./cmd/eed snapshot list --json
go run ./cmd/eed doctor
go run ./cmd/eed verify --json
go run ./cmd/eed install --dry-run
go run ./cmd/eed test-drive
go run ./cmd/eed test-drive --json
```

## Commands

- `eed version` prints the application version.
- `eed version --json` emits build metadata as JSON.
- `eed detect` prints a human-readable read-only system report, including distribution version/codename, CPU and RAM, GPUs and driver binding, block devices, mounted filesystems, init/services, and package managers.
- `eed detect --json` emits the detection report as JSON to stdout.
- `eed wizard` runs the interactive configuration wizard.
- `eed config show` prints the active TOML configuration.
- `eed config validate` validates the current configuration.
- `eed plan` generates a dependency-aware, non-destructive installation plan.
- `eed plan --json` emits the plan as JSON to stdout.
- `eed assess` evaluates compatibility and resources for the current config and host.
- `eed assess --json` emits the assessment as JSON to stdout.
- `eed storage analyze` inspects filesystems, mount points, and capacity in read-only mode.
- `eed storage analyze --json` emits the storage report as JSON.
- `eed snapshot status` probes the existing Snapper backend and readiness status.
- `eed snapshot list` lists snapshots from the configured Snapper backend.
- `eed snapshot create --config root --description "Before EED operation"` creates an explicitly requested read-only EED-owned snapshot using an existing Snapper configuration.
- `eed verify` checks selected components using trusted built-in checks.
- `eed verify --json` emits a versioned verification report as JSON.
- `eed verify --component developer.docker` verifies one configured component.
- `eed doctor` runs read-only system and component diagnostics.
- `eed doctor --json` emits the doctor report as JSON.
- `eed install --dry-run` builds a read-only integrated preview and reports unavailable execution stages.
- `eed install` does not modify the system and exits with an explanation that installation engines are not implemented.
- `eed test-drive` interactively displays the read-only installation preview, diagnostics, and proposed summary; its choices never start installation or change configuration.
- `eed test-drive --json` emits the same report as JSON without prompting.

## Storage and snapshot safety

The storage subsystem is intentionally diagnostic and non-destructive. It reads `/proc/self/mountinfo` and unprivileged filesystem statistics without changing the host. Free blocks and blocks available to the current user are reported separately. Optional Btrfs metadata failures are warnings and do not discard valid capacity data. LVM-specific metadata inspection is not currently implemented and does not affect basic filesystem capacity reporting. It deliberately does not create or delete subvolumes, modify `/etc/fstab`, enable quotas, or perform rollback operations.

Snapshot creation is only supported through an existing Snapper configuration and requires an explicit elevated invocation when used on a real system. The backend will never silently auto-create configurations or delete user-owned snapshots.

`eed test-drive` does not create snapshots. Its report distinguishes unsupported filesystems (including ext4 for the configured Btrfs/Snapper protection), missing Snapper, missing backend configuration, unsupported creation, and unverified full rollback. A configured backend is not evidence that a snapshot was created or that rollback works.

Storage inspection skips pseudo-filesystems and non-root container overlay mounts, and groups inaccessible optional mounts into a summary. A failure to inspect `/` remains an explicit blocking storage error; no `sudo` is used.

## Verification and recovery status

The Verification Engine runs only trusted, built-in checks. Unselected components are reported as skipped. Current checks inspect executable availability/version, Docker daemon reachability, basic Hyprland configuration presence, GPU detection when Hyprland is selected, and host detection. These are diagnostics, not a package inventory or a guarantee that a component is fully functional.

The Arch adapter provides read-only installed-package inventory and cached repository metadata inspection, plus a tested full-upgrade transaction builder/executor that is not wired to the public installer. On Ubuntu/Debian, Test Drive uses `dpkg-query` and `apt-cache` for read-only inventory and cached candidate metadata; it never runs `apt update`, `install`, or `upgrade`. Package estimates remain unknown when the local metadata does not establish a size. Neither backend is invoked to install packages by Test Drive. Ubuntu diagnostics do not imply Ubuntu installation support: the planner and installer remain Arch-targeted. Configuration deployment, Service Manager, EED ownership manifests, and recovery journals are still missing. Accordingly, `doctor` reports recovery availability as unknown. Component recovery, uninstall, and full-system snapshot rollback are not implemented; no recovery readiness is implied.

## Release status

EED 1.0.0 is not release-ready. The repository has no Arch VM test results, user-facing package transaction integration, configuration/service deployment engines, or safe rollback/recovery manager. Provider estimates are based only on cached Pacman package/file metadata; stale, incomplete, or ambiguous provider metadata remains unresolved. See [docs/RELEASE_AUDIT.md](./docs/RELEASE_AUDIT.md), [docs/SECURITY_AUDIT.md](./docs/SECURITY_AUDIT.md), and the [manual VM test matrix](./docs/testing/VM_TEST_MATRIX.md).

The project owner selected Apache-2.0; see [LICENSE](./LICENSE). The license choice does not make the current development build release-ready.

## Customization and registry

The customization layer normalizes user config into a desired state and resolves desktop, editor, theme, and developer tool selections without making system changes.

Component metadata is stored as TOML manifests under `components/manifests/` and embedded into the binary as built-in data. Resource discovery uses this order: `--data-dir`, `EED_DATA_DIR`, embedded manifests, `/usr/share/eed`, then development-root discovery using `go.mod` and `components/manifests` markers. A configured directory can be the manifest directory itself or contain `components/manifests`; a bad explicit configuration returns an error instead of silently falling back. This lets `eed` run from the repository root, `dist/`, or an unrelated working directory without relying on the executable's relative location.

Each manifest includes a unique ID, supported OS and architecture, dependencies, conflicts, package names, compatibility requirements, and optional resource requirements. The registry validates duplicate IDs, malformed documents, missing dependencies, and cycles.

The Tokyo Night manifest currently records the selection but declares no install packages; the Test Drive reports this without substituting another theme.

Manifests can provide package-level download and installed sizes, architecture, version, metadata source/date, confidence, and runtime package dependencies. Estimates include the selected component dependency closure and deduplicate packages. Some selected x86_64 packages have versioned Arch repository metadata; totals remain partial when transitive dependency sizes or a component footprint are unavailable. Test Drive reports missing metadata instead of guessing. Available estimates are approximate because repository versions and installed package state can differ.

## Installation planner

The planner receives the normalized configuration, manifest registry, and current detection report. It expands component dependency chains, selects desktop-specific package variants, and reports warnings or blockers. On Arch, Test Drive reads installed packages and cached official repository metadata, reports package states, and estimates total/additional sizes. Runtime dependencies are resolved separately from optional and build dependencies; soname and virtual providers are checked against cached package metadata, with the file database queried when ownership is needed. Pacman is never asked to synchronize databases or install packages during Test Drive. On Ubuntu/Debian, it selects the APT/dpkg diagnostic adapter and reads installed versions plus cached candidate metadata without refreshing repository databases. Ambiguous or unavailable providers and missing sizes remain unknown; incomplete metadata is reported, not guessed.

GPU assessment reports PCI identity separately from vendor names, virtual/physical status, driver binding, DRM/render-node availability, OpenGL renderer, and acceleration/software-rendering status. A Virtio or QXL device is available for diagnostics and package planning, but does not by itself prove 3D acceleration. Resource assessment uses the selected component requirements and distinguishes minimum total RAM, recommended total RAM, and currently available RAM. Snapshot assessment identifies the root filesystem and read-only Btrfs/LVM layout facts; Snapper presence is not proof of rollback. On a virtualized host, Test Drive may recommend a hypervisor-managed external VM snapshot, which is not an EED-managed filesystem snapshot.

The plan is intentionally diagnostic. `installable` is set only when the current host is Arch Linux and the plan has no blockers.

Test Drive distinguishes the host assessment (`blocked`, `needs_review`, `diagnostic_only`, or `ready`) from the plan's installability and from unavailable installation engines. Ubuntu remains non-installable and diagnostic-only for the Arch target. `ready` describes host checks only, not installation readiness; package, configuration, service, and recovery engines remain unimplemented.

## Detection Engine

The detection subsystem is intentionally read-only and segmented into:

- hardware detection
- system detection
- software detection

GPU inventory uses sysfs PCI vendor/device IDs and optional `lspci` enrichment; driver binding is reported separately. Package-manager detection reports only the distribution-native manager and the EED Arch target (`pacman`), avoiding warnings about unrelated managers.

See [docs/detection.md](./docs/detection.md) for the architecture, output schema, safety guarantees and limitations.

## Safety

Detection, planning, assessment, verification, doctor, installation preview, and Test Drive are read-only. The Pacman transaction executor exists only inside the adapter and is not exposed by the public CLI; integrating it into an installer would require a one-use explicit approval, an already-elevated process, Pacman's own interactive confirmation, and `-Syu --needed` transactions. The public CLI's only host-changing operation is explicitly invoked read-only Snapper snapshot creation against an existing config with elevated privileges; it does not deploy configuration, modify services, change `/etc`, or alter bootloader configuration. Installation, configuration/service deployment, uninstall, recovery, and rollback execution are not implemented.
# eed-linux
