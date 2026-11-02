# Disposable Arch Linux VM test matrix

These are manual test plans, not completed test results. Run only in disposable QEMU virtual machines with known-good VM snapshots. Never use a daily-use machine for filesystem, package transaction, or rollback testing.

| ID | Guest profile | Storage / resources | Expected checks |
|---|---|---|---|
| VM-01 | Minimal Arch TTY | ext4, 2 vCPU, 2 GiB RAM | Detection works; GUI components remain unselected; no Snapper rollback claim. |
| VM-02 | Minimal Arch TTY | Btrfs root with prepared Snapper root config | Read-only storage/snapshot status; manually verify mount topology and config target. Do not run rollback until dedicated preflight exists. |
| VM-03 | KDE Plasma | ext4, 4 vCPU, 4 GiB RAM | Diagnostic commands work; selected desktop checks report actual executable/config states. |
| VM-04 | XFCE | ext4, 2 vCPU, 3 GiB RAM | Selected desktop check; unselected desktops are skipped. |
| VM-05 | Hyprland | Btrfs or ext4, 4 vCPU, 4 GiB RAM | Binary/config presence diagnostics. QEMU does not validate physical GPU driver behavior. |
| VM-06 | Minimal Arch | Small disk with low free space | Storage metrics are reported; incomplete estimates remain unknown; no transaction begins. |
| VM-07 | Minimal Arch | 1 GiB RAM | Resource assessment reports measured/unknown values; no install action. |
| VM-08 | Unsupported Ubuntu | ext4 | Detection and diagnostics remain available; installation preview identifies unsupported target and never substitutes APT. |

## Procedure and result recording

For each VM, record the QEMU configuration, guest OS/kernel, disk and filesystem/subvolume layout, EED version/commit, selected config, commands and exit status, JSON reports, relevant guest logs, and limitations. Mark results `MANUALLY VERIFIED` only after a human executes and reviews the scenario. CI does not validate any VM row.

Do not execute snapshot creation or rollback as part of the current matrix: the project does not yet have rollback preflight or disposable-VM automation. Any future destructive integration test must require an explicit separate opt-in and approval.
