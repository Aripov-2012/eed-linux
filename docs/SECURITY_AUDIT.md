# Security audit

**Status:** development review; not a security certification. The repository is not ready for a stable release.

## Scope reviewed

CLI process boundaries, config write path, component manifests, detection runners, storage read paths, Snapper operations, verification checks, and the newly added read-only install preview. No privileged operation or destructive test was run.

## Findings

### High — Snapshot readiness is not rollback readiness

The Snapper probe verifies the presence of the selected named configuration but does not establish that it targets the running root subvolume, that Btrfs default-subvolume behavior is compatible, or that fstab/kernel command-line/bootloader layout is safe. The capability intentionally keeps `SupportsRollback` false. Do not use `ready` as approval to roll back.

**Mitigation:** no rollback command is exposed; documentation and reports state that rollback readiness is not established. A complete boot-layout preflight and disposable-VM test remain release blockers.

### Medium — Snapper output parsing depends on textual table layout

Configuration and snapshot listing parsers consume human-oriented delimited output rather than a versioned machine format. Locale/format changes can make metadata inaccurate.

**Mitigation:** snapshots are only classified as EED-owned from explicit `eed=true` userdata; descriptions alone do not prove ownership. Replace the parsers with a stable supported format if Snapper provides one, and test fixtures against supported Snapper versions.

### Medium — Snapshot creation was not validated against real Snapper/Btrfs

The call path is covered by mocks, not an Arch VM. Config-target suitability, actual readonly state, Snapper argument compatibility, and exact userdata persistence have not been independently validated.

**Mitigation:** creation requires an existing named config, elevated execution, requests read-only, and checks the created number/ownership marker after listing. It remains experimental until disposable-VM verification.

### Medium — External command timeouts and diagnostics are inconsistent

Some detection paths call external tools with background contexts or do not expose stderr independently. A hung optional utility can delay detection, and failures may lack useful diagnostic detail.

**Mitigation:** the verification runner uses context-aware commands and Btrfs/Snapper calls have bounded contexts where wired. Normalize runner contracts and timeout/error reporting before stable release.

### Medium — No ownership/baseline data for system mutations

Package, host-configuration, and service mutation APIs and ownership manifests are absent. Removing or restoring based only on current state could affect user-managed resources.

**Mitigation:** no install execution, uninstall, recovery, or rollback commands are implemented. The install preview is read-only and declares these stages blocked.

## Positive controls observed

- Command execution uses argument arrays and does not construct shell strings in the reviewed paths.
- Detection and install preview are read-only.
- Config saving uses a same-directory temporary file and atomic rename.
- Component manifests contain data, not executable scripts.
- Snapper creation rejects non-elevated and writable requests, verifies the named existing config, applies a timeout, requests `--read-only`, and verifies an EED userdata marker after creation.
- Unit tests use fake command runners; ordinary test suite does not execute real package, service, snapshot, or rollback operations.

## Release decision

No claim of zero vulnerabilities is made. The findings and missing subsystems above prevent a stable 1.0.0 security conclusion. Repeat a focused security review after package/configuration/service mutation and recovery APIs are implemented, before any system-changing release.
