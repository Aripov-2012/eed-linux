package servicemanager

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// Journal tracks applied service actions for rollback and diagnostics.
type Journal struct {
	mu      sync.Mutex
	path    string
	Entries []JournalEntry `json:"entries"`
}

func NewJournal(path string) (*Journal, error) {
	j := &Journal{
		path:    filepath.Clean(path),
		Entries: []JournalEntry{},
	}
	if err := j.load(); err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("load service journal from %s: %w", path, err)
	}
	return j, nil
}

func (j *Journal) load() error {
	data, err := os.ReadFile(j.path)
	if err != nil {
		return err
	}
	if len(data) == 0 {
		return nil
	}
	return json.Unmarshal(data, &j.Entries)
}

func (j *Journal) Save() error {
	j.mu.Lock()
	defer j.mu.Unlock()

	data, err := json.MarshalIndent(j.Entries, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal service journal entries: %w", err)
	}

	dir := filepath.Dir(j.path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return fmt.Errorf("create service journal directory %q: %w", dir, err)
	}

	tmpFile := fmt.Sprintf("%s.tmp.%d", j.path, os.Getpid())
	if err := os.WriteFile(tmpFile, data, 0o600); err != nil {
		return fmt.Errorf("write temporary service journal file %q: %w", tmpFile, err)
	}

	if err := os.Rename(tmpFile, j.path); err != nil {
		_ = os.Remove(tmpFile)
		return fmt.Errorf("commit service journal file %q: %w", j.path, err)
	}

	return nil
}

func (j *Journal) AddEntry(entry JournalEntry) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.Entries = append(j.Entries, entry)
}

func (j *Journal) UpdateStatus(opID, unit string, scope Scope, status string) {
	j.mu.Lock()
	defer j.mu.Unlock()

	for i := range j.Entries {
		if j.Entries[i].OperationID == opID && j.Entries[i].UnitName == unit && j.Entries[i].Scope == scope {
			j.Entries[i].Status = status
		}
	}
}

func (j *Journal) EntriesByOperation(opID string) []JournalEntry {
	j.mu.Lock()
	defer j.mu.Unlock()

	var result []JournalEntry
	for _, entry := range j.Entries {
		if entry.OperationID == opID {
			result = append(result, entry)
		}
	}
	return result
}
