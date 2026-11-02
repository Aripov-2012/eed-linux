# EED — Linux Detection Engine Refinement

## Objective

Improve Linux hardware, storage, package-manager and service detection based on the latest real Ubuntu 24.04 test.

Preserve all existing working functionality.

Do not implement package installation or modify the user's operating system.

## BUG 1 — Block Device Classification

Current output incorrectly classifies /dev/loopN devices as ordinary disks.

Example:
loop0: /dev/loop0 (disk, ...)
loop1: /dev/loop1 (disk, ...)

Requirements:

- Correctly distinguish physical disks, partitions, loop devices, device-mapper devices and other virtual block devices.
- Detect NVMe, SATA and USB storage where possible.
- Do not classify loop devices as physical disks.
- Do not treat Snap loop devices as independent physical storage.
- Preserve accurate disk and partition capacity.
- Prefer reliable Linux device metadata such as sysfs and lsblk information when available.
- Do not require root privileges.

Add unit tests for each device type.

## BUG 2 — Mounted Filesystem Classification

The latest output lists dozens of Snap squashfs mounts and other special mountpoints individually.

Improve mountpoint classification.

Categories:
- Physical/local filesystems.
- EFI and boot filesystems.
- Snap squashfs mounts.
- Pseudo filesystems.
- Docker/container overlays.
- FUSE/GVFS.
- Namespace mounts.
- Other virtual filesystems.

Requirements:

- Show root, home, boot and EFI mounts clearly.
- Group repeated Snap mounts.
- Group pseudo filesystems.
- Avoid confusing read-only Snap images with independent storage devices.
- Preserve meaningful capacity information for real filesystems.
- Never silently discard actual user data filesystems.

Investigate the suspicious entry:

unknown: on /home

Determine whether /home is a separate mount or belongs to the root filesystem.

If it is not an independent mount, do not invent a separate filesystem entry.

Use reliable mount information and test bind mounts, nested mounts and ordinary directories.

Add tests for:
- Root filesystem.
- Separate /home partition.
- Bind-mounted /home.
- EFI partition.
- Snap mounts.
- Docker overlays.
- FUSE.
- Pseudo filesystems.

## BUG 3 — Package Manager Status Semantics

Current package-manager output uses contradictory descriptions.

Separate these concepts explicitly:

- Host distribution.
- Host-native package manager.
- Package-manager availability.
- Configured target distribution.
- Target package manager.
- Adapter implementation status.

For Ubuntu:
- APT must be identified as native and available.
- Pacman must not be described as native to Ubuntu.
- If Arch is the configured target, report Pacman as the target package manager, not as the native manager of Ubuntu.

For Arch:
- Pacman must be identified as native.
- APT must not be described as native.

Do not infer distribution identity solely from executable presence.

Add tests for Ubuntu, Debian, Arch and a system with multiple installed package-manager executables.

## BUG 4 — Service Detection

Improve service discovery.

Requirements:

- Detect init system.
- Distinguish system and user services.
- Detect active, inactive, failed and unavailable units.
- Discover the active display-manager service through display-manager.service where supported.
- Support service aliases.
- Do not hardcode only SDDM and greetd.
- Detect GNOME display-manager configuration where present.
- Avoid reporting unrelated inactive services as active services.
- Explain the difference between installed, loaded and active.
- Preserve read-only behavior.

Do not start, stop, enable or disable any service.

Test systemd alias resolution and system/user service distinction.

## OUTPUT QUALITY

Improve output readability.

Example device summary:

Physical disks:
  - nvme0n1: 477 GiB

Partitions:
  - nvme0n1p1: EFI
  - nvme0n1p2: ext4, mounted at /

Virtual devices:
  - Loop devices: 39 (primarily Snap)

Mounted filesystems:
  - Root: ext4
  - EFI: vfat
  - Snap mounts: grouped
  - Pseudo filesystems: grouped

Avoid excessive output by default while retaining a detailed diagnostic mode.

Also fix formatting issues such as missing whitespace in GPU driver descriptions.

## TESTING

Add unit and integration tests.

Use mock system command execution where possible.

Do not modify the current Ubuntu installation.

Test Drive and detect must remain read-only.

Run:
- gofmt
- go test ./...
- go vet ./...
- go test -race ./...

## DELIVERABLES

Provide:
1. Modified files.
2. Confirmed bugs.
3. Detection accuracy improvements.
4. New test coverage.
5. Test results.
6. Remaining limitations.

Preserve existing Arch backend functionality.

Do not claim universal Linux compatibility solely because several distribution names can be detected.