# EED release audit

**Audit scope:** repository source, public CLI, configuration/manifests, current tests, Makefile, documentation, and release files present during the EED 1.0.0 preparation pass.

**Decision:** NOT READY FOR RELEASE CANDIDATE. This is a source review and local test/build review, not an Arch VM validation or a certification of system-changing behavior. No release artifact is published or claimed.

## Subsystem inventory

| Subsystem | Classification | Evidence and limitations |
|---|---|---|
| CLI foundation | PARTIALLY_IMPLEMENTED | Cobra commands exist for version, wizard, config show/validate, detect, plan, assess, storage, snapshot, verify, doctor, and read-only install preview. Several commands specified for config diff/apply/status, packages, services, recovery, uninstall, and rollback are correctly absent because their engines are missing. CLI integration tests are sparse. |
| Configuration (EED preferences) | IMPLEMENTED_AND_TESTED | TOML load/default/validate/save logic and tests exist. This is not a host configuration deployment engine and does not track ownership/backups. |
| Detection | IMPLEMENTED_AND_TESTED | Hardware/system/software reports and tests exist. Some optional command detection is incomplete and its command runner/timeout behavior is not uniform. |
| Customization | IMPLEMENTED_AND_TESTED | Config normalization and component selection are implemented with tests. |
| Component Registry | IMPLEMENTED_AND_TESTED | TOML manifests, dependency validation, cycle detection, and tests exist. Manifests do not define safe executable verification logic. |
| Installation Planner | IMPLEMENTED_AND_TESTED | Dependency-aware diagnostic plan, desktop-specific package variants, and tests exist. Arch Test Drive enriches it with installed-package states and cached repository metadata; provider resolution can remain incomplete. It does not install packages. |
| Compatibility Engine | IMPLEMENTED_AND_TESTED | Read-only compatibility checks and tests exist; input facts can remain unknown or incomplete. |
| Resource Analyzer | IMPLEMENTED_AND_TESTED | CPU/RAM/storage estimates and tests exist. Package sizes are optional/incomplete; available storage aggregation needs further topology-aware validation. |
| Assessment Engine | IMPLEMENTED_AND_TESTED | Aggregates compatibility/resource outputs and has tests. It is not a transaction gate or a system mutation safety proof. |
| Storage Analyzer | PARTIALLY_IMPLEMENTED | Mountinfo/Statfs analysis and parser tests exist. Device topology/EFI identification is heuristic. Btrfs/LVM support is partial; nested-subvolume coverage helper is not integrated into the main report. |
| Btrfs analysis | PARTIALLY_IMPLEMENTED | Optional command wrappers, output parsers, and a coverage helper exist. No supported-layout VM verification; exclusive snapshot usage and quota state are not reliably determined. |
| LVM analysis | PARTIALLY_IMPLEMENTED | Parsing helpers exist, but comprehensive PV/VG/LV/thin-pool collection and validation are absent. No LVM snapshot creation support. |
| Snapshot/Snapper | PARTIALLY_IMPLEMENTED | Capability/config listing and read-only snapshot creation command path exist. Creation now requires elevated execution, forces read-only, checks the named config, uses timeouts, and verifies a listed EED userdata marker. Text output parsing remains brittle; target-layout validation and real VM integration tests are absent. Rollback is not implemented. |
| Arch package adapter | PARTIALLY_IMPLEMENTED | Read-only `pacman -Q`/`-Qe` inventory and cached `-Si` metadata inspection are integrated into Test Drive. The adapter can build and execute only an explicitly approved `-Syu --needed` transaction without `--noconfirm`, but this path is not wired to the CLI. There is no package ownership journal, no real transaction test, and virtual dependency provider resolution may remain unknown. |
| Host Configuration Engine | MISSING | No deployment templates, ownership manifests, backup/restore API, or hash conflict workflow. Existing config package only stores EED preferences. |
| Service Manager | MISSING | Detection can inspect a limited set of services. No plan/apply, scope separation, before/after state capture, or ownership tracking. |
| Verification Engine | PARTIALLY_IMPLEMENTED | Registry engine and mock tests exist. Current built-ins are targeted executable/host/config-presence checks; not package inventory, config hash/permissions, or broad service verification. |
| Recovery Manager | MISSING | No recovery plans, safe restore/uninstall, ownership manifests, or recovery journal. |
| Installation Orchestrator | PARTIALLY_IMPLEMENTED | Added a read-only preflight preview integrating config, detection, registry, planner, assessment, storage, and Snapper capability. It does not execute installation, confirm, create a baseline, write journals, or persist final state. |
| Installation journal/state | MISSING | No installation state directory, atomic journal, integrity validation, or migration policy. |
| Build/CI | PARTIALLY_IMPLEMENTED | Make targets and a read-only GitHub Actions workflow were added. Workflow has not run on GitHub in this session; no lint/release-publish workflow or release verification exists. |
| Documentation | PARTIALLY_IMPLEMENTED | README, detection/verification docs, release/security audits, changelog, contributor and VM-matrix docs are present. Package/config/service/recovery behavior and actual VM results remain unavailable. |
| License | IMPLEMENTED_BUT_UNTESTED | Project owner selected Apache-2.0 and supplied the copyright holder. `LICENSE` was added. This resolves the license-selection blocker only; it is not legal advice. |

## Integration gaps and inconsistencies

- Detection, verification, Snapper, and storage have different command-runner abstractions. Storage command calls are not consistently injectable.
- JSON schema versions mix integer and string types. Status vocabularies also vary by subsystem (`ready`, `blocked`, lowercase assessment labels, uppercase verification states).
- The system-detection storage list and the dedicated storage analyzer overlap but are not unified.
- Package candidates are compared against installed package versions on Arch; other hosts report the Pacman inventory as not applicable.
- Cached package dependency metadata includes runtime dependencies, optional dependencies, provides, conflicts, replacements, architecture, and sizes. A provider for a virtual dependency is not discovered by `pacman -Si <name>` alone and remains an explicit unknown.
- The integrated install preview reports package inventory and size limitations; configuration-file and service-change details remain unknown.
- The Snapper capability probe confirms the selected named configuration, not boot-compatible rollback readiness.
- The Btrfs coverage helper is not connected to the main analyzer and must not be used as evidence of complete coverage.
- Build and unit-test success does not establish a safe package transaction, configuration deployment, service operation, or rollback.

## Release blockers

1. Implement and test Package Manager, host Configuration, and Service Manager APIs with ownership/baseline tracking.
2. Implement an execution orchestrator, explicit confirmations, cancellation/locking, and a durable journal.
3. Implement Recovery Manager only after ownership manifests exist.
4. Implement Btrfs/Snapper rollback preflight and execution or explicitly defer it; no current rollback capability exists.
5. Connect storage subvolume/boot-layout analysis to reports and validate actual supported layouts.
6. Test actual snapshot creation in a disposable Arch+Btrfs+Snapper VM.
7. Run the entire required CI checks and manual VM matrix, recording results.
8. Complete CLI and documentation tests and review release packaging on disposable Arch.
9. Resolve the security limitations documented in `SECURITY_AUDIT.md`.

## Arch backend audit (source and mock-test scope)

- No existing `pacman -Sy <package>`, partial-upgrade installation, automatic `--noconfirm`, or installation call from Test Drive was found. The new transaction builder emits only `-Syu --needed -- <targets>`, requests no removal, requires an exact one-use EED approval, and leaves Pacman's native confirmation enabled. The executor is not called by the public CLI.
- Inventory and metadata inspection use read-only `-Q`, `-Qe`, `-Si`, and `-Qo` queries. The sync database timestamp is inspected without refreshing it; stale/missing metadata marks estimates incomplete. Mock tests verify no transaction or sync command occurs during inventory.
- Package states distinguish installed, absent, version mismatch, repository-unavailable, and unknown. Installed dependency versions are considered in additional-size estimates. Unknown dependency names may indicate missing metadata or unresolved virtual providers; no false provider/package-unavailable claim is made for those transitive names.
- `system.portals` selects Hyprland+GTK, GNOME, KDE, or GTK generic packages by desktop. The portal planner detects desktop-specific and generic user config files and emits only a proposal; it never writes or backs up files. Config deployment, backup/ownership tracking, and actual portal-session validation remain unimplemented.
- Code - OSS is identified explicitly as the edition provided by Arch's official `code` package.

## Local validation

The development environment has Go 1.24.0 on Linux/amd64. Local `go test ./...` and `go vet ./...` passed after the Arch adapter changes. No QEMU scenario, real Arch package transaction, release publication, or physical GPU test was performed.
