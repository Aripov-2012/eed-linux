package wizard

import (
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func TestWizardBuildConfig(t *testing.T) {
	m := NewModel("/tmp/eed.toml")
	m.SingleSelected[0] = 0
	m.SingleSelected[1] = 1
	m.SingleSelected[3] = 1
	m.SingleSelected[4] = 1
	m.MultiSelected[2] = map[string]bool{"Git": true, "Docker": true, "Python": true}

	cfg := m.BuildConfig()
	if cfg.Profile.Purposes[0] != "development" {
		t.Fatalf("unexpected profile purpose: %s", cfg.Profile.Purposes[0])
	}
	if cfg.Desktop.Environment != "kde" {
		t.Fatalf("unexpected desktop environment: %s", cfg.Desktop.Environment)
	}
	if !cfg.Developer.Git || !cfg.Developer.Docker || !cfg.Developer.Python {
		t.Fatal("expected selected developer tools to be enabled")
	}
	if cfg.Editor.Name != "vscode" {
		t.Fatalf("unexpected editor: %s", cfg.Editor.Name)
	}
	if cfg.Appearance.Theme != "tokyo-night" {
		t.Fatalf("unexpected appearance theme: %s", cfg.Appearance.Theme)
	}
}

func TestWizardStateTransitions(t *testing.T) {
	m := NewModel("/tmp/eed.toml")
	updated, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	next, ok := updated.(Model)
	if !ok {
		t.Fatal("unexpected wizard model type")
	}
	if next.SingleSelected[0] != 1 {
		t.Fatalf("expected selection to move down to index 1, got %d", next.SingleSelected[0])
	}

	updated, _ = next.Update(tea.KeyMsg{Type: tea.KeyEnter})
	next, _ = updated.(Model)
	if next.CurrentStep != 1 {
		t.Fatalf("expected current step to advance to 1, got %d", next.CurrentStep)
	}
}
