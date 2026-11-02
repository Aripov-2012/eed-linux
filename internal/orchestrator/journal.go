package orchestrator

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"eed/internal/adapters/arch/pacman"
	"eed/internal/configmanager"
	"eed/internal/servicemanager"
	"eed/internal/verification"
)

// InstallationJournalRecord tracks the complete end-to-end state of an installation run.
type InstallationJournalRecord struct {
	SchemaVersion       string                          `json:"schema_version"`
	OperationID         string                          `json:"operation_id"`
	StartedAt           time.Time                       `json:"started_at"`
	CompletedAt         time.Time                       `json:"completed_at"`
	Profile             string                          `json:"profile"`
	HostOS              string                          `json:"host_os"`
	TargetOS            string                          `json:"target_os"`
	SelectedComponents  []string                        `json:"selected_components"`
	SnapshotID          int                             `json:"snapshot_id,omitempty"`
	SnapshotDescription string                          `json:"snapshot_description,omitempty"`
	PacmanTransaction   *pacman.TransactionRecord       `json:"pacman_transaction,omitempty"`
	ConfigurationReport *configmanager.DeploymentReport `json:"configuration_report,omitempty"`
	ServiceResult       *servicemanager.ServiceResult   `json:"service_result,omitempty"`
	VerificationReport  *verification.Report            `json:"verification_report,omitempty"`
	OverallStatus       string                          `json:"overall_status"` // "success", "completed_with_warnings", "failed", "aborted"
	Errors              []string                        `json:"errors,omitempty"`
	Warnings            []string                        `json:"warnings,omitempty"`
}

// JournalStorage manages saving and querying installation records.
type JournalStorage struct {
	Directory string
}

func DefaultJournalStorage() *JournalStorage {
	dir := "/var/lib/eed/installations"
	// Check if writable or fallback to user state directory
	if err := os.MkdirAll(dir, 0o755); err != nil {
		home, _ := os.UserHomeDir()
		if home == "" {
			home = os.Getenv("HOME")
		}
		dir = filepath.Join(home, ".local", "state", "eed", "installations")
		_ = os.MkdirAll(dir, 0o700)
	}
	return &JournalStorage{Directory: dir}
}

func (s *JournalStorage) Save(record InstallationJournalRecord) error {
	if record.OperationID == "" {
		record.OperationID = fmt.Sprintf("eed-install-%d", time.Now().UnixNano())
	}
	record.SchemaVersion = "1"

	dir := s.Directory
	if dir == "" {
		dir = DefaultJournalStorage().Directory
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create installation journal directory %q: %w", dir, err)
	}

	data, err := json.MarshalIndent(record, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal installation journal record: %w", err)
	}

	targetPath := filepath.Join(dir, record.OperationID+".json")
	tmpPath := fmt.Sprintf("%s.tmp.%d", targetPath, os.Getpid())

	if err := os.WriteFile(tmpPath, data, 0o600); err != nil {
		return fmt.Errorf("write temporary installation journal %q: %w", tmpPath, err)
	}

	if err := os.Rename(tmpPath, targetPath); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("commit installation journal %q: %w", targetPath, err)
	}

	// Update latest pointer
	latestPath := filepath.Join(dir, "latest.json")
	latestTmp := fmt.Sprintf("%s.tmp.%d", latestPath, os.Getpid())
	if err := os.WriteFile(latestTmp, data, 0o600); err == nil {
		_ = os.Rename(latestTmp, latestPath)
	}

	return nil
}

func (s *JournalStorage) LoadLatest() (*InstallationJournalRecord, error) {
	dir := s.Directory
	if dir == "" {
		dir = DefaultJournalStorage().Directory
	}

	latestPath := filepath.Join(dir, "latest.json")
	data, err := os.ReadFile(latestPath)
	if err != nil {
		return nil, err
	}

	var record InstallationJournalRecord
	if err := json.Unmarshal(data, &record); err != nil {
		return nil, fmt.Errorf("unmarshal latest journal %q: %w", latestPath, err)
	}

	return &record, nil
}
