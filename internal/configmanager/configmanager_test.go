package configmanager

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDeployNewFile(t *testing.T) {
	tempDir := t.TempDir()
	configDir := filepath.Join(tempDir, ".config")
	journalPath := filepath.Join(tempDir, "state", "journal.json")

	mgr, err := NewManager(Options{
		ApprovedRoots: []string{configDir},
		JournalPath:   journalPath,
		UserConfigDir: configDir,
		UserHome:      tempDir,
	})
	if err != nil {
		t.Fatal(err)
	}

	dest := filepath.Join(configDir, "app", "config.toml")
	content := []byte("setting = true\n")

	items := []Item{
		{
			Kind:        KindFile,
			ComponentID: "test.app",
			Destination: dest,
			Content:     content,
		},
	}

	report, err := mgr.Deploy(context.Background(), "op-1", items, false)
	if err != nil {
		t.Fatalf("unexpected deploy error: %v", err)
	}
	if !report.Success || len(report.Deployed) != 1 {
		t.Fatalf("expected 1 deployed file: %+v", report)
	}

	read, err := os.ReadFile(dest)
	if err != nil {
		t.Fatal(err)
	}
	if string(read) != string(content) {
		t.Fatalf("expected content %q, got %q", string(content), string(read))
	}

	// Verify journal
	latest := mgr.Journal.FindLatestEntry(dest)
	if latest == nil || latest.Status != "deployed" || latest.DeployedHash == "" {
		t.Fatalf("expected journal entry: %+v", latest)
	}
}

func TestDeployExistingUnmanagedFileRequiresApprovalAndBacksUp(t *testing.T) {
	tempDir := t.TempDir()
	configDir := filepath.Join(tempDir, ".config")
	journalPath := filepath.Join(tempDir, "state", "journal.json")

	mgr, err := NewManager(Options{
		ApprovedRoots: []string{configDir},
		JournalPath:   journalPath,
		UserConfigDir: configDir,
		UserHome:      tempDir,
	})
	if err != nil {
		t.Fatal(err)
	}

	dest := filepath.Join(configDir, "app", "existing.conf")
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		t.Fatal(err)
	}
	existingContent := []byte("original user setting\n")
	if err := os.WriteFile(dest, existingContent, 0o644); err != nil {
		t.Fatal(err)
	}

	newContent := []byte("new eed setting\n")
	items := []Item{
		{
			Kind:        KindFile,
			ComponentID: "test.app",
			Destination: dest,
			Content:     newContent,
		},
	}

	// Deployment without approval must fail!
	_, err = mgr.Deploy(context.Background(), "op-2", items, false)
	if err == nil || !strings.Contains(err.Error(), "requires explicit user approval") {
		t.Fatalf("expected explicit approval error, got: %v", err)
	}

	// Original file must not be modified
	read, _ := os.ReadFile(dest)
	if string(read) != string(existingContent) {
		t.Fatalf("file was overwritten without approval: %q", string(read))
	}

	// Deployment with approval should create backup and replace
	report, err := mgr.Deploy(context.Background(), "op-2", items, true)
	if err != nil {
		t.Fatalf("approved deployment failed: %v", err)
	}
	if !report.Success || len(report.Backups) != 1 {
		t.Fatalf("expected 1 backup created, got: %+v", report)
	}

	// Verify backup content
	backupData, err := os.ReadFile(report.Backups[0])
	if err != nil {
		t.Fatalf("read backup failed: %v", err)
	}
	if string(backupData) != string(existingContent) {
		t.Fatalf("backup content mismatch: got %q, want %q", string(backupData), string(existingContent))
	}

	// Verify new content installed
	readNew, _ := os.ReadFile(dest)
	if string(readNew) != string(newContent) {
		t.Fatalf("new content mismatch: got %q", string(readNew))
	}
}

func TestDeploySafeSymlink(t *testing.T) {
	tempDir := t.TempDir()
	configDir := filepath.Join(tempDir, ".config")
	journalPath := filepath.Join(tempDir, "state", "journal.json")

	mgr, err := NewManager(Options{
		ApprovedRoots: []string{configDir, tempDir},
		JournalPath:   journalPath,
		UserConfigDir: configDir,
		UserHome:      tempDir,
	})
	if err != nil {
		t.Fatal(err)
	}

	target := filepath.Join(tempDir, "source.conf")
	if err := os.WriteFile(target, []byte("target file"), 0o644); err != nil {
		t.Fatal(err)
	}

	linkDest := filepath.Join(configDir, "link.conf")
	items := []Item{
		{
			Kind:          KindSymlink,
			ComponentID:   "test.symlink",
			Destination:   linkDest,
			SymlinkTarget: target,
		},
	}

	report, err := mgr.Deploy(context.Background(), "op-sym", items, false)
	if err != nil {
		t.Fatalf("deploy symlink failed: %v", err)
	}
	if !report.Success {
		t.Fatalf("expected success: %+v", report)
	}

	resolved, err := os.Readlink(linkDest)
	if err != nil {
		t.Fatalf("readlink failed: %v", err)
	}
	if resolved != target {
		t.Fatalf("expected target %q, got %q", target, resolved)
	}

	// Re-deploy identical symlink should skip cleanly
	plan, err := mgr.Plan(items)
	if err != nil {
		t.Fatal(err)
	}
	if plan[0].Action != "skip_identical" {
		t.Fatalf("expected skip_identical, got %q", plan[0].Action)
	}
}

func TestPathTraversalPrevention(t *testing.T) {
	tempDir := t.TempDir()
	configDir := filepath.Join(tempDir, ".config")

	mgr, err := NewManager(Options{
		ApprovedRoots: []string{configDir},
		UserConfigDir: configDir,
		UserHome:      tempDir,
	})
	if err != nil {
		t.Fatal(err)
	}

	invalidDests := []string{
		filepath.Join(configDir, "..", "..", "etc", "passwd"),
		"/etc/shadow",
		"relative/path/config",
		filepath.Join(configDir, "../unapproved/file"),
	}

	for _, dest := range invalidDests {
		items := []Item{{Kind: KindFile, Destination: dest, Content: []byte("bad")}}
		_, err := mgr.Plan(items)
		if err == nil {
			t.Errorf("expected destination %q to be rejected for path traversal / approval root", dest)
		}
	}
}

func TestRollbackRestoresBackupAndPreservesUserModification(t *testing.T) {
	tempDir := t.TempDir()
	configDir := filepath.Join(tempDir, ".config")
	journalPath := filepath.Join(tempDir, "state", "journal.json")

	mgr, err := NewManager(Options{
		ApprovedRoots: []string{configDir},
		JournalPath:   journalPath,
		UserConfigDir: configDir,
		UserHome:      tempDir,
	})
	if err != nil {
		t.Fatal(err)
	}

	destNew := filepath.Join(configDir, "app", "new.conf")
	destExisting := filepath.Join(configDir, "app", "existing.conf")

	if err := os.MkdirAll(filepath.Dir(destExisting), 0o755); err != nil {
		t.Fatal(err)
	}
	origExisting := []byte("original content\n")
	if err := os.WriteFile(destExisting, origExisting, 0o644); err != nil {
		t.Fatal(err)
	}

	items := []Item{
		{Kind: KindFile, ComponentID: "c1", Destination: destNew, Content: []byte("new file\n")},
		{Kind: KindFile, ComponentID: "c2", Destination: destExisting, Content: []byte("replacement\n")},
	}

	report, err := mgr.Deploy(context.Background(), "op-rb", items, true)
	if err != nil {
		t.Fatal(err)
	}
	if !report.Success {
		t.Fatal("deployment failed")
	}

	// User modifies destNew after deployment
	userEdit := []byte("user edited this file!\n")
	if err := os.WriteFile(destNew, userEdit, 0o644); err != nil {
		t.Fatal(err)
	}

	// Rollback
	if err := mgr.Rollback(context.Background(), "op-rb"); err != nil {
		t.Fatalf("rollback failed: %v", err)
	}

	// destExisting should be restored to origExisting
	restored, err := os.ReadFile(destExisting)
	if err != nil {
		t.Fatalf("read restored existing file: %v", err)
	}
	if string(restored) != string(origExisting) {
		t.Fatalf("restored content mismatch: got %q, want %q", string(restored), string(origExisting))
	}

	// destNew was modified by user: it should NOT have been silently deleted!
	// It should either still exist or have been renamed to .user_modified
	entries, _ := os.ReadDir(filepath.Dir(destNew))
	foundUserModified := false
	for _, e := range entries {
		if strings.Contains(e.Name(), "user_modified") || e.Name() == filepath.Base(destNew) {
			content, _ := os.ReadFile(filepath.Join(filepath.Dir(destNew), e.Name()))
			if string(content) == string(userEdit) {
				foundUserModified = true
				break
			}
		}
	}
	if !foundUserModified {
		t.Fatal("user modified file was silently deleted during rollback!")
	}
}
