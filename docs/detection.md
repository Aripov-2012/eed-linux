# Detection Engine

The EED Detection Engine is a read-only subsystem that inspects the running host without modifying the operating system.

## Architecture

The detection engine is split into three independent modules:

- Hardware Detection: CPU, memory, GPU, battery, BIOS, device kind.
- System Detection: OS, kernel, init system, boot mode, session, virtualization, storage.
- Software Detection: package managers, common commands, relevant services.

A central `detection.Detect` function orchestrates each module and returns a unified report.

GPU inventory follows PCI device symlinks in sysfs, identifies vendors from PCI IDs (including Intel `0x8086`), and can enrich model and bound-driver details with optional `lspci -Dnnk` output. GPU presence is separate from kernel-driver binding. Block devices are classified using sysfs paths and names, including partitions, loop devices, device mapper, and NVMe/SATA/USB transports when available. Loop and Snap-backed storage is not presented as physical disks.

Storage inventory comes from `/proc/self/mountinfo` and unprivileged filesystem statistics. Root, home, boot, EFI, bind, local, remote, Snap, overlay, FUSE, namespace, and pseudo-filesystem mounts receive explicit categories. Only actual mountinfo entries are reported; an ordinary home directory is not invented as a filesystem. `free_bytes` reflects filesystem free blocks, while `available_bytes` reflects blocks available to the current user. Capacity is not attributed to Snap images, pseudo-filesystems, overlays, or namespace mounts; local and remote data filesystems retain their reported capacity. Failures for individual mount points or optional Btrfs metadata do not discard capacity retrieved from other mounts.

Package-manager reporting keeps the detected host distribution and its native manager separate from the configured Arch target, target manager, executable availability, and adapter implementation status. Distribution identity comes from OS metadata, not from which package-manager executables happen to be installed. Service reporting uses read-only `systemctl` queries, distinguishes installed/loaded/active state and system/user scope, and resolves `display-manager.service` aliases when available.

Pseudo-filesystems and non-root container overlay mounts are categorized and skipped for capacity checks. Inaccessible optional mounts are reported in one summarized group with a small sample of paths; a capacity failure on `/` remains an explicit error and is not downgraded to an optional warning. Detection never invokes `sudo`.

Component size estimates are based only on package metadata supplied in manifests. They track download and installed sizes separately, can follow declared runtime package dependencies, and deduplicate package names across selected component dependencies. Some selected x86_64 packages have versioned Arch repository metadata; totals remain partial when dependency sizes or a component footprint are unavailable. Missing metadata is reported as unavailable rather than replaced with a guessed number.

## Output schema

The final report includes:

- schema_version
- collected_at
- system
- hardware
- software
- warnings

The report is machine-readable JSON and is intended for future EED subsystems such as planner or compatibility checks.

## Missing vs unknown vs unavailable

- Missing: the value is absent because the source file is not present or not available.
- Unknown: the system was inspected but the value could not be identified reliably.
- Unavailable: a feature is not offered in the current environment or requires a platform-specific system dependency not present on the host.

Examples include:

- OS release file missing -> missing
- desktop environment not detected -> unknown
- `systemctl` absent -> unavailable

## CLI usage

```bash
eed detect
eed detect --json
eed detect --detailed
```

The default mode emits a concise human-readable summary, grouping repeated special mounts. `--detailed` lists every detected block device and mount; JSON mode includes the full categorized report and avoids non-report output on stdout.

## Safety guarantees

This engine is strictly read-only. It does not:

- install or remove packages
- modify `/etc`
- alter system services
- create snapshots
- update bootloader configuration
- touch user dotfiles
- run commands through `sudo`

## Known limitations

- Some desktop environment values are inferred from environment variables and may be `unknown` if context is not present.
- GPU detection relies primarily on sysfs; optional `lspci` enrichment may be absent on minimal systems.
- non-standard or minimal Linux systems may not expose all expected data sources.
- service detection is only supported when `systemctl` is available.
