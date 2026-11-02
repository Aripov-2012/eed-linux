package configmanager

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"text/template"
	"time"
)

type Options struct {
	ApprovedRoots []string
	JournalPath   string
	UserHome      string
	UserConfigDir string
}

type ConfigManager struct {
	ApprovedRoots []string
	Journal       *Journal
	UserHome      string
	UserConfigDir string
}

func NewManager(opts Options) (*ConfigManager, error) {
	home := opts.UserHome
	if home == "" {
		var err error
		home, err = os.UserHomeDir()
		if err != nil {
			home = os.Getenv("HOME")
		}
	}

	configDir := opts.UserConfigDir
	if configDir == "" {
		configDir = filepath.Join(home, ".config")
	}

	approvedRoots := opts.ApprovedRoots
	if len(approvedRoots) == 0 {
		approvedRoots = []string{
			configDir,
			home,
			"/etc",
		}
	}

	journalPath := opts.JournalPath
	if journalPath == "" {
		journalPath = filepath.Join(home, ".local", "state", "eed", "config_journal.json")
	}

	journal, err := NewJournal(journalPath)
	if err != nil {
		return nil, fmt.Errorf("initialize configuration journal: %w", err)
	}

	return &ConfigManager{
		ApprovedRoots: approvedRoots,
		Journal:       journal,
		UserHome:      home,
		UserConfigDir: configDir,
	}, nil
}

// Plan evaluates a list of configuration items against the actual filesystem and journal state.
func (m *ConfigManager) Plan(items []Item) ([]PlanItem, error) {
	planItems := make([]PlanItem, 0, len(items))

	for _, item := range items {
		cleanDest, err := ValidateDestinationPath(item.Destination, m.ApprovedRoots)
		if err != nil {
			return nil, fmt.Errorf("validate destination for component %q: %w", item.ComponentID, err)
		}
		item.Destination = cleanDest

		planItem := PlanItem{
			Item:     item,
			Warnings: []string{},
		}

		info, lstatErr := os.Lstat(cleanDest)
		if lstatErr != nil {
			if os.IsNotExist(lstatErr) {
				planItem.Exists = false
				planItem.DestinationType = "none"
				planItem.Action = "create"
				planItem.RequiresApproval = false
				planItems = append(planItems, planItem)
				continue
			}
			return nil, fmt.Errorf("lstat %q: %w", cleanDest, lstatErr)
		}

		planItem.Exists = true
		mode := info.Mode()

		switch {
		case mode&os.ModeSymlink != 0:
			planItem.DestinationType = "symlink"
			target, readErr := os.Readlink(cleanDest)
			if readErr == nil && item.Kind == KindSymlink && target == item.SymlinkTarget {
				planItem.Action = "skip_identical"
			} else {
				planItem.Action = "backup_and_replace"
				planItem.RequiresApproval = true
				planItem.BackupPath = fmt.Sprintf("%s.eed.bak.%d", cleanDest, time.Now().UnixNano())
			}
		case mode.IsDir():
			planItem.DestinationType = "directory"
			if item.Kind == KindDirectory {
				planItem.Action = "skip_identical"
			} else {
				return nil, fmt.Errorf("destination %q is a directory; cannot overwrite with %s", cleanDest, item.Kind)
			}
		case mode.IsRegular():
			planItem.DestinationType = "regular"
			currentHash, hashErr := fileSHA256(cleanDest)
			if hashErr != nil {
				return nil, fmt.Errorf("hash existing file %q: %w", cleanDest, hashErr)
			}

			// Render content if template to compare
			desiredContent := item.Content
			if item.Kind == KindTemplate {
				rendered, tmplErr := renderTemplate(item.TemplateString, nil)
				if tmplErr == nil {
					desiredContent = rendered
				}
			}

			desiredHash := hashBytes(desiredContent)
			if desiredHash != "" && desiredHash == currentHash {
				planItem.Action = "skip_identical"
				planItem.RequiresApproval = false
				planItems = append(planItems, planItem)
				continue
			}

			latest := m.Journal.FindLatestEntry(cleanDest)
			if latest != nil && latest.Status == "deployed" {
				planItem.EEDOwned = true
				if latest.DeployedHash != "" && latest.DeployedHash != currentHash {
					planItem.ModifiedByUser = true
					planItem.RequiresApproval = true
					planItem.Action = "backup_and_replace"
					planItem.BackupPath = fmt.Sprintf("%s.eed.bak.%d", cleanDest, time.Now().UnixNano())
					planItem.Warnings = append(planItem.Warnings, fmt.Sprintf("file %q was modified by user since EED deployment", cleanDest))
				} else {
					planItem.Action = "update"
					planItem.BackupPath = fmt.Sprintf("%s.eed.bak.%d", cleanDest, time.Now().UnixNano())
				}
			} else {
				// Unmanaged existing file
				planItem.EEDOwned = false
				planItem.RequiresApproval = true
				planItem.Action = "backup_and_replace"
				planItem.BackupPath = fmt.Sprintf("%s.eed.bak.%d", cleanDest, time.Now().UnixNano())
				planItem.Warnings = append(planItem.Warnings, fmt.Sprintf("unmanaged existing configuration file %q will be backed up before replacement", cleanDest))
			}
		default:
			planItem.DestinationType = "other"
			return nil, fmt.Errorf("destination %q has unsupported special file mode: %v", cleanDest, mode)
		}

		planItems = append(planItems, planItem)
	}

	return planItems, nil
}

// Deploy executes the configuration deployment for the given items according to safety rules.
func (m *ConfigManager) Deploy(ctx context.Context, opID string, items []Item, approved bool) (DeploymentReport, error) {
	if opID == "" {
		opID = fmt.Sprintf("cfg-%d", time.Now().UnixNano())
	}

	report := DeploymentReport{
		OperationID: opID,
		Timestamp:   time.Now().UTC(),
		Deployed:    []JournalEntry{},
		Backups:     []string{},
		Errors:      []string{},
		Warnings:    []string{},
	}

	plan, err := m.Plan(items)
	if err != nil {
		report.Success = false
		report.Errors = append(report.Errors, err.Error())
		return report, err
	}

	// Verify approval requirement
	for _, p := range plan {
		if p.RequiresApproval && !approved {
			err := fmt.Errorf("configuration deployment requires explicit user approval: file %q is an unmanaged or user-modified file", p.Item.Destination)
			report.Success = false
			report.Errors = append(report.Errors, err.Error())
			return report, err
		}
	}

	// Execute deployment
	for _, p := range plan {
		if err := ctx.Err(); err != nil {
			report.Success = false
			report.Errors = append(report.Errors, err.Error())
			return report, err
		}

		dest := p.Item.Destination
		if p.Action == "skip_identical" {
			continue
		}

		// Handle backup if replacing
		if p.BackupPath != "" && p.Exists {
			if err := createBackup(dest, p.BackupPath, p.DestinationType); err != nil {
				report.Success = false
				errDetail := fmt.Sprintf("create backup for %q: %v", dest, err)
				report.Errors = append(report.Errors, errDetail)
				return report, errors.New(errDetail)
			}
			report.Backups = append(report.Backups, p.BackupPath)
		}

		var deployedHash string
		var symlinkTarget string

		switch p.Item.Kind {
		case KindDirectory:
			dirMode := os.FileMode(0o755)
			if p.Item.FileMode != 0 {
				dirMode = os.FileMode(p.Item.FileMode)
			}
			if err := os.MkdirAll(dest, dirMode); err != nil {
				report.Success = false
				report.Errors = append(report.Errors, err.Error())
				return report, fmt.Errorf("create directory %q: %w", dest, err)
			}

		case KindSymlink:
			symlinkTarget = p.Item.SymlinkTarget
			if err := atomicSymlink(p.Item.SymlinkTarget, dest); err != nil {
				report.Success = false
				report.Errors = append(report.Errors, err.Error())
				return report, fmt.Errorf("create safe symlink %q -> %q: %w", dest, p.Item.SymlinkTarget, err)
			}

		case KindFile, KindTemplate:
			content := p.Item.Content
			if p.Item.Kind == KindTemplate {
				rendered, err := renderTemplate(p.Item.TemplateString, nil)
				if err != nil {
					report.Success = false
					report.Errors = append(report.Errors, err.Error())
					return report, fmt.Errorf("render template for %q: %w", dest, err)
				}
				content = rendered
			}

			mode := os.FileMode(0o644)
			if p.Item.FileMode != 0 {
				mode = os.FileMode(p.Item.FileMode)
			}

			if err := atomicWriteFile(dest, content, mode); err != nil {
				report.Success = false
				report.Errors = append(report.Errors, err.Error())
				return report, fmt.Errorf("deploy file %q atomically: %w", dest, err)
			}
			deployedHash = hashBytes(content)
		}

		entry := JournalEntry{
			OperationID:   opID,
			Timestamp:     time.Now().UTC(),
			ComponentID:   p.Item.ComponentID,
			Kind:          p.Item.Kind,
			Destination:   dest,
			Source:        p.Item.Source,
			BackupPath:    p.BackupPath,
			DeployedHash:  deployedHash,
			SymlinkTarget: symlinkTarget,
			Status:        "deployed",
		}

		m.Journal.AddEntry(entry)
		report.Deployed = append(report.Deployed, entry)
	}

	if err := m.Journal.Save(); err != nil {
		report.Warnings = append(report.Warnings, fmt.Sprintf("save journal: %v", err))
	}

	report.Success = true
	return report, nil
}

func atomicWriteFile(dest string, content []byte, mode os.FileMode) error {
	dir := filepath.Dir(dest)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create parent directory %q: %w", dir, err)
	}

	tmpFile, err := os.CreateTemp(dir, ".eed-tmp-*")
	if err != nil {
		return fmt.Errorf("create temp file in %q: %w", dir, err)
	}
	tmpName := tmpFile.Name()

	if _, err := tmpFile.Write(content); err != nil {
		tmpFile.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("write temp file: %w", err)
	}

	if err := tmpFile.Chmod(mode); err != nil {
		tmpFile.Close()
		_ = os.Remove(tmpName)
		return fmt.Errorf("chmod temp file: %w", err)
	}

	if err := tmpFile.Close(); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("close temp file: %w", err)
	}

	if err := os.Rename(tmpName, dest); err != nil {
		_ = os.Remove(tmpName)
		return fmt.Errorf("atomic rename %q -> %q: %w", tmpName, dest, err)
	}

	return nil
}

func atomicSymlink(target, dest string) error {
	dir := filepath.Dir(dest)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create parent directory %q: %w", dir, err)
	}

	tmpLink := filepath.Join(dir, fmt.Sprintf(".eed-symlink-%d", time.Now().UnixNano()))
	if err := os.Symlink(target, tmpLink); err != nil {
		return fmt.Errorf("create temp symlink %q -> %q: %w", tmpLink, target, err)
	}

	if err := os.Rename(tmpLink, dest); err != nil {
		_ = os.Remove(tmpLink)
		return fmt.Errorf("atomic rename symlink %q -> %q: %w", tmpLink, dest, err)
	}

	return nil
}

func createBackup(src, backupPath, destType string) error {
	backupDir := filepath.Dir(backupPath)
	if err := os.MkdirAll(backupDir, 0o700); err != nil {
		return fmt.Errorf("create backup directory: %w", err)
	}

	if destType == "symlink" {
		target, err := os.Readlink(src)
		if err != nil {
			return err
		}
		return os.Symlink(target, backupPath)
	}

	srcFile, err := os.Open(src)
	if err != nil {
		return err
	}
	defer srcFile.Close()

	info, err := srcFile.Stat()
	if err != nil {
		return err
	}

	dstFile, err := os.OpenFile(backupPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, info.Mode())
	if err != nil {
		return err
	}
	defer dstFile.Close()

	if _, err := io.Copy(dstFile, srcFile); err != nil {
		return err
	}

	return dstFile.Sync()
}

func renderTemplate(tmplText string, data any) ([]byte, error) {
	tmpl, err := template.New("config").Parse(tmplText)
	if err != nil {
		return nil, fmt.Errorf("parse template: %w", err)
	}
	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, data); err != nil {
		return nil, fmt.Errorf("execute template: %w", err)
	}
	return buf.Bytes(), nil
}

func fileSHA256(path string) (string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return hashBytes(data), nil
}

func hashBytes(data []byte) string {
	if data == nil {
		return ""
	}
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}
