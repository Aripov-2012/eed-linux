# Arch Linux VM Testing Instructions for EED

This document provides step-by-step instructions for testing the EED installation engine inside a disposable Arch Linux Virtual Machine (QEMU / KVM).

> [!CAUTION]
> Never test real Pacman transactions, configuration replacements, or service modifications on your host system. All real installation tests must only take place inside a disposable Arch Linux VM with clean external VM snapshots.

---

## 1. Prerequisites and VM Setup

### Recommended VM Specifications:
- **Hypervisor:** QEMU/KVM (`virt-manager`, `quickemu`, or command-line QEMU)
- **CPUs:** 2–4 vCPUs
- **Memory:** 4 GiB RAM
- **Disk:** 20–40 GiB
- **Filesystem Setup (Two Test Scenarios):**
  1. **Btrfs + Snapper Scenario:** Root on Btrfs subvolume `@`, subvolume `@home` for `/home`, and Snapper configured for `root`.
  2. **ext4 Scenario:** Standard ext4 layout to verify that unverified/unprotected snapshot states warn and prompt correctly.

### Example QEMU Launch Command (Using an Arch Cloud Image or Installed Disk):
```bash
qemu-system-x86_64 \
  -enable-kvm \
  -m 4G \
  -smp 4 \
  -drive file=arch-disposable.qcow2,if=virtio,format=qcow2 \
  -net nic,model=virtio \
  -net user,hostfwd=tcp::2222-:22 \
  -display none -vga virtio -daemonize
```

---

## 2. Transferring EED to the Arch VM

From your development host (Ubuntu), build the EED binary and transfer it along with the component manifests:

```bash
# On development host:
go build -o dist/eed ./cmd/eed

# Copy binary and manifests to Arch VM:
scp -P 2222 dist/eed archuser@localhost:~/eed
scp -P 2222 -r components archuser@localhost:~/components
```

Log in to the VM as a standard non-root user with `sudo` privileges:
```bash
ssh -p 2222 archuser@localhost
chmod +x ~/eed
```

---

## 3. Test Scenarios and Step-by-Step Validation

### Scenario A: Read-Only Guarantees (Test Drive & Dry-Run)
Verify that preview and test-drive modes do not mutate any packages, files, or services:

1. **Test Drive execution:**
   ```bash
   ./eed test-drive
   ```
   *Expected Outcome:* Read-only inspection runs without prompting for sudo or executing mutations.

2. **Dry-Run execution:**
   ```bash
   ./eed install --dry-run
   ```
   *Expected Outcome:* Full installation plan is generated; all candidate packages, portal configs, and services are previewed; no system changes are made.

---

### Scenario B: Snapshot Protection Preflight
1. **On an ext4 system (or Btrfs without Snapper):**
   ```bash
   ./eed install
   ```
   *Expected Outcome:* EED detects that snapshot protection is unverified/unavailable:
   ```
   WARNING: Snapshot protection is unavailable (...).
   Type PROCEED to continue without snapshot protection:
   ```
   Typing anything other than `PROCEED` immediately aborts the installation safely without executing any commands.

2. **Continuing with `--allow-unprotected`:**
   ```bash
   ./eed install --allow-unprotected
   ```
   *Expected Outcome:* Bypasses the snapshot confirmation prompt and directly presents the transaction confirmation.

---

### Scenario C: Package Transaction Engine & Terminal Handoff Validation
1. **Transaction Confirmation:**
   EED prints the proposed transaction:
   ```
   Proposed Installation Summary:
   - Profile: development
   - Full System Upgrade: Pacman will synchronize repositories and perform a full system upgrade.
   - Package candidates: git, code, hyprland, ...
   Type YES to authorize EED to start the installation:
   ```
   Typing `NO` or pressing Enter aborts cleanly. Typing `YES` starts elevated Pacman execution.

2. **Terminal Handoff Notice & Interactive Confirmation:**
   Right before Pacman executes, EED announces terminal handoff:
   ```
   EED is handing terminal control to Pacman.
   ```
   Pacman synchronizes databases, presents its package list and sizes, and prompts:
   ```
   Proceed with installation? [Y/n]
   ```
   - **Keyboard input verification:** Enter `Y` or `n`. Ensure input reaches Pacman and keyboard strokes are not swallowed.
   - **Rejection scenario:** Entering `n` causes Pacman to abort with code 1. EED restores the terminal and reports:
     ```
     Transaction cancelled.
     Exit status: 1
     Verification status: not_run
     ```
   - **Cancellation scenario:** Pressing `Ctrl+C` cleanly aborts with code 130 and reports `Transaction cancelled.`
   - **Confirmation scenario:** Entering `Y` proceeds with installation, downloads packages, and reports:
     ```
     Transaction completed.
     Exit status: 0
     Verification status: verified
     ```

3. **Safe Full System Upgrade Strategy:**
   Verify in process tree / logs that EED invoked:
   ```bash
   sudo pacman -Syu --needed -- <packages...>
   ```
   - No unsafe `pacman -Sy` was run.
   - Pacman's native dependency resolver, conflict resolution, and signature verification were honored.
   - Only approved targets were requested.

4. **Non-TTY Safe Failure Test:**
   Run `./eed install < /dev/null`.
   EED must reject execution before starting the transaction:
   `installation requires an interactive terminal to confirm package transactions; standard input is not a terminal`

5. **Ownership Tracking:**
   After transaction, verify that the journal at `~/.local/state/eed/installations/` recorded:
   - Operation ID
   - Timestamp
   - Host & Target OS
   - Selected components
   - Explicit package targets
   - Newly installed packages tracked by EED (`InstalledByEED`)

---

### Scenario D: Configuration Manager Safety & User Modification Preservation
1. **Initial Deployment:**
   - EED deploys portal configurations to `~/.config/xdg-desktop-portal/`.
   - Verify permissions and file contents.

2. **Preserving User Modifications:**
   - Edit the deployed configuration file manually:
     ```bash
     echo "# Manual user tweak" >> ~/.config/xdg-desktop-portal/portals.conf
     ```
   - Re-run `./eed install`.
   - Verify that EED does not overwrite the modified file without backup or approval. If modified, the user modification is backed up / preserved, and ownership is tracked.

---

### Scenario E: Systemd Service Manager Validation
1. **User and System Scope Separation:**
   - User services are configured via `systemctl --user`.
   - System services are configured via `sudo systemctl --system`.
2. **State Verification:**
   - Check that only requested unit files were enabled/started.
   - Non-existent unit files trigger clear warnings without panicking.
   - State before and after is persisted in `~/.local/state/eed/service_journal.json`.

---

### Scenario F: Post-Installation Verification & Recovery
1. **Run Verification:**
   ```bash
   ./eed verify
   ./eed doctor
   ```
   *Expected Outcome:*
   - All installed packages are queried via `pacman -Q`.
   - Configuration file paths, checksums, and symlink targets are validated.
   - Service states (`active`, `enabled`) are confirmed.
   - Active recovery mechanism is identified from the installation journal.

---

## 4. Resetting the Disposable VM

After testing, revert the VM to its pre-test base snapshot:
```bash
# On development host:
qemu-img snapshot -a clean-base arch-disposable.qcow2
```
