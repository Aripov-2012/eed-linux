package wizard

import (
	"fmt"
	"os"
	"strings"

	eedconfig "eed/internal/config"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

type Step struct {
	Title       string
	Options     []string
	MultiSelect bool
}

type Model struct {
	Steps          []Step
	CurrentStep    int
	SingleSelected map[int]int
	MultiSelected  map[int]map[string]bool
	configPath     string
	Saved          bool
	Canceled       bool
}

func NewModel(path string) Model {
	steps := []Step{
		{Title: "System usage", Options: []string{"Development", "Everyday use", "Gaming", "Minimal system", "Multimedia", "Other"}},
		{Title: "Desktop environment", Options: []string{"GNOME", "KDE Plasma", "XFCE", "Hyprland", "Sway", "i3", "Keep existing environment", "No graphical environment"}},
		{Title: "Developer tools", Options: []string{"Git", "Docker", "Python", "Node.js", "Go", "Rust"}, MultiSelect: true},
		{Title: "Preferred editor", Options: []string{"Neovim", "VS Code", "None"}},
		{Title: "Appearance", Options: []string{"Catppuccin", "Tokyo Night", "Other / default"}},
	}

	return Model{
		Steps:          steps,
		SingleSelected: map[int]int{0: 0, 1: 0, 3: 0, 4: 0},
		MultiSelected:  map[int]map[string]bool{2: {}},
		configPath:     path,
	}
}

func (m Model) Init() tea.Cmd {
	return nil
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "ctrl+c", "q":
			m.Canceled = true
			return m, tea.Quit
		case "esc":
			if m.CurrentStep > 0 {
				m.CurrentStep--
				return m, nil
			}
			m.Canceled = true
			return m, tea.Quit
		case "up", "k":
			m.moveSelection(-1)
			return m, nil
		case "down", "j":
			m.moveSelection(1)
			return m, nil
		case "left", "b":
			if m.CurrentStep > 0 {
				m.CurrentStep--
			}
			return m, nil
		case "right":
			if m.CurrentStep < len(m.Steps)-1 {
				m.CurrentStep++
			}
			return m, nil
		case "space":
			if m.Steps[m.CurrentStep].MultiSelect {
				m.toggleCurrentOption()
			}
			return m, nil
		case "enter":
			if m.CurrentStep == len(m.Steps)-1 {
				m.Saved = true
				return m, tea.Quit
			}
			m.CurrentStep++
			return m, nil
		}
	}
	return m, nil
}

func (m Model) View() string {
	step := m.Steps[m.CurrentStep]
	var builder strings.Builder

	builder.WriteString(lipgloss.NewStyle().Bold(true).Render("EED configuration wizard"))
	builder.WriteString("\n\n")
	builder.WriteString(lipgloss.NewStyle().Bold(true).Render(step.Title))
	builder.WriteString("\n")

	for idx, option := range step.Options {
		selected := false
		prefix := "  ◯ "
		if step.MultiSelect {
			if m.MultiSelected[m.CurrentStep][option] {
				selected = true
				prefix = "  ◉ "
			}
		} else {
			if m.SingleSelected[m.CurrentStep] == idx {
				selected = true
				prefix = "  ● "
			}
		}
		if selected {
			builder.WriteString(lipgloss.NewStyle().Bold(true).Render(prefix + option))
		} else {
			builder.WriteString(prefix + option)
		}
		builder.WriteString("\n")
	}

	builder.WriteString("\n")
	builder.WriteString("Up/Down: move | Enter: continue | Space: toggle | Esc: cancel | b: back\n")

	if m.CurrentStep == len(m.Steps)-1 {
		builder.WriteString("\nFinal review: press Enter to save the configuration to the XDG config directory.\n")
	}

	return builder.String()
}

func (m *Model) moveSelection(delta int) {
	if m.Steps[m.CurrentStep].MultiSelect {
		return
	}

	max := len(m.Steps[m.CurrentStep].Options) - 1
	selected := m.SingleSelected[m.CurrentStep]
	selected += delta
	if selected < 0 {
		selected = 0
	}
	if selected > max {
		selected = max
	}
	m.SingleSelected[m.CurrentStep] = selected
}

func (m *Model) toggleCurrentOption() {
	step := m.CurrentStep
	if m.MultiSelected[step] == nil {
		m.MultiSelected[step] = map[string]bool{}
	}

	selectedIndex := m.SingleSelected[step]
	if selectedIndex >= len(m.Steps[step].Options) {
		selectedIndex = 0
	}
	option := m.Steps[step].Options[selectedIndex]
	m.MultiSelected[step][option] = !m.MultiSelected[step][option]
}

func (m Model) Persist() error {
	if !m.Saved {
		return nil
	}
	if _, err := os.Stat(m.configPath); err == nil {
		return fmt.Errorf("configuration already exists at %q; remove it before saving a new configuration", m.configPath)
	}
	cfg := m.BuildConfig()
	if err := eedconfig.SaveConfig(m.configPath, cfg, false); err != nil {
		return fmt.Errorf("save wizard config: %w", err)
	}
	return nil
}

func (m Model) BuildConfig() eedconfig.Config {
	cfg := eedconfig.DefaultConfig()

	if usageChoice, ok := m.currentSingleOption(0); ok {
		cfg.Profile.Name = strings.ToLower(strings.ReplaceAll(usageChoice, " ", "-"))
		cfg.Profile.Purposes = []string{mapUsageChoice(usageChoice)}
	}

	if envChoice, ok := m.currentSingleOption(1); ok {
		cfg.Desktop.Environment = mapDesktopChoice(envChoice)
		cfg.Desktop.DisplayManager = mapDisplayManagerChoice(envChoice)
	}

	if selected, ok := m.MultiSelected[2]; ok {
		cfg.Developer.Git = selected["Git"]
		cfg.Developer.Docker = selected["Docker"]
		cfg.Developer.Python = selected["Python"]
		cfg.Developer.NodeJS = selected["Node.js"]
		cfg.Developer.Go = selected["Go"]
		cfg.Developer.Rust = selected["Rust"]
	}

	if editorChoice, ok := m.currentSingleOption(3); ok {
		cfg.Editor.Name = mapEditorChoice(editorChoice)
	}

	if appearanceChoice, ok := m.currentSingleOption(4); ok {
		cfg.Appearance.Theme = mapAppearanceChoice(appearanceChoice)
	}

	return cfg
}

func (m Model) currentSingleOption(step int) (string, bool) {
	idx, ok := m.SingleSelected[step]
	if !ok {
		return "", false
	}
	if step < 0 || step >= len(m.Steps) {
		return "", false
	}
	if idx < 0 || idx >= len(m.Steps[step].Options) {
		return "", false
	}
	return m.Steps[step].Options[idx], true
}

func mapUsageChoice(choice string) string {
	switch choice {
	case "Development":
		return "development"
	case "Everyday use":
		return "everyday"
	case "Gaming":
		return "gaming"
	case "Minimal system":
		return "minimal"
	case "Multimedia":
		return "multimedia"
	default:
		return "other"
	}
}

func mapDesktopChoice(choice string) string {
	switch choice {
	case "GNOME":
		return "gnome"
	case "KDE Plasma":
		return "kde"
	case "XFCE":
		return "xfce"
	case "Hyprland":
		return "hyprland"
	case "Sway":
		return "sway"
	case "i3":
		return "i3"
	case "Keep existing environment":
		return "existing"
	default:
		return "none"
	}
}

func mapDisplayManagerChoice(choice string) string {
	switch choice {
	case "GNOME":
		return "gdm"
	case "KDE Plasma":
		return "sddm"
	case "XFCE":
		return "lightdm"
	case "Keep existing environment":
		return "existing"
	default:
		return "none"
	}
}

func mapEditorChoice(choice string) string {
	switch choice {
	case "Neovim":
		return "neovim"
	case "VS Code":
		return "vscode"
	default:
		return "none"
	}
}

func mapAppearanceChoice(choice string) string {
	switch choice {
	case "Catppuccin":
		return "catppuccin-mocha"
	case "Tokyo Night":
		return "tokyo-night"
	default:
		return "default"
	}
}
