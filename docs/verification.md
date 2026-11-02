# Verification Engine

`eed verify` and `eed doctor` run read-only, trusted built-in checks through a registry. Component manifests cannot provide executable checks or arbitrary commands.

## Statuses

- `PASS`: the check succeeded.
- `WARNING`: a limitation or unavailable optional capability was detected.
- `FAIL`: a selected component's required executable or configuration prerequisite is missing.
- `UNKNOWN`: available evidence is insufficient.
- `SKIPPED`: the component is not selected in the active EED configuration.

The report includes a schema version, aggregate counts, component-level results, and a recovery availability field. A failed check produces a non-zero command result; JSON mode still writes only the report to stdout.

## Current checks

- Selected Git, Docker, Python, Node.js, Go, Rust, Neovim, and VS Code executable/version checks.
- Docker daemon reachability without starting or enabling a service.
- Selected desktop executable checks for GNOME, KDE, XFCE, Hyprland, Sway, and i3.
- Presence-only checking of the conventional Hyprland user configuration path.
- Detection Engine GPU information when Hyprland is selected.
- Operating-system and kernel information.

Package ownership/version comparison, configuration hash/permission verification, syntax validation, and general service-state verification are unavailable because their respective manager/tracking APIs do not exist in this repository yet. The current checks must not be interpreted as a complete package or configuration audit.

## Recovery

Recovery is not implemented. There are no EED ownership manifests or journals from which safe restoration could be proven. Configuration restore, service restore, package uninstall, and system rollback must remain unavailable until those tracking APIs exist. In particular, a Snapper snapshot is not by itself proof that full rollback is boot-compatible.

## CLI

```bash
eed verify
eed verify --json
eed verify --component developer.docker
eed doctor
eed doctor --json
```

All commands are read-only and do not install packages, change services, rewrite configuration, or restore snapshots.
