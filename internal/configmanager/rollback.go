package configmanager

import (
	"context"
	"fmt"
	"os"
	"time"
)

// Rollback restores configurations from backups and safely removes newly created files.
func (m *ConfigManager) Rollback(ctx context.Context, opID string) error {
	entries := m.Journal.EntriesByOperation(opID)
	if len(entries) == 0 {
		return fmt.Errorf("no journal entries found for operation ID %q", opID)
	}

	// Iterate in reverse deployment order
	for i := len(entries) - 1; i >= 0; i-- {
		if err := ctx.Err(); err != nil {
			return err
		}

		entry := entries[i]
		if entry.Status == "rolled_back" {
			continue
		}

		dest := entry.Destination
		if entry.BackupPath != "" {
			// Restore backup
			if _, err := os.Lstat(entry.BackupPath); err == nil {
				// Atomic restore backup to original destination
				info, err := os.Lstat(entry.BackupPath)
				if err == nil && info.Mode()&os.ModeSymlink != 0 {
					target, readErr := os.Readlink(entry.BackupPath)
					if readErr == nil {
						_ = os.Remove(dest)
						_ = atomicSymlink(target, dest)
					}
				} else {
					data, readErr := os.ReadFile(entry.BackupPath)
					if readErr == nil {
						_ = atomicWriteFile(dest, data, info.Mode())
					}
				}
				_ = os.Remove(entry.BackupPath)
			}
		} else {
			// Newly created file/symlink
			info, err := os.Lstat(dest)
			if err == nil {
				if info.Mode().IsRegular() {
					currentHash, _ := fileSHA256(dest)
					if currentHash != entry.DeployedHash && currentHash != "" {
						// User modified the file after installation! Never silently delete it!
						preservePath := fmt.Sprintf("%s.user_modified.%d", dest, time.Now().UnixNano())
						_ = os.Rename(dest, preservePath)
					} else {
						_ = os.Remove(dest)
					}
				} else {
					_ = os.Remove(dest)
				}
			}
		}

		m.Journal.UpdateStatus(opID, dest, "rolled_back")
	}

	return m.Journal.Save()
}
