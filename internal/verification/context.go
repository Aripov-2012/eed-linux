package verification

import (
	"context"
	"strings"

	"eed/internal/config"
	"eed/internal/detection"
)

type Context struct {
	Runner    Runner
	Detection detection.Report
	Config    config.Config
}

func (c Context) IsSelected(componentID string) bool {
	id := strings.ToLower(strings.TrimSpace(componentID))
	switch id {
	case "system":
		return true
	case "recovery":
		return true
	case "developer.git":
		return c.Config.Developer.Git
	case "developer.docker":
		return c.Config.Developer.Docker
	case "developer.python":
		return c.Config.Developer.Python
	case "developer.nodejs":
		return c.Config.Developer.NodeJS
	case "developer.go":
		return c.Config.Developer.Go
	case "developer.rust":
		return c.Config.Developer.Rust
	case "desktop.gnome":
		return strings.EqualFold(c.Config.Desktop.Environment, "gnome")
	case "desktop.kde":
		return strings.EqualFold(c.Config.Desktop.Environment, "kde") || strings.EqualFold(c.Config.Desktop.Environment, "plasma")
	case "desktop.xfce":
		return strings.EqualFold(c.Config.Desktop.Environment, "xfce")
	case "desktop.hyprland":
		return strings.EqualFold(c.Config.Desktop.Environment, "hyprland")
	case "desktop.sway":
		return strings.EqualFold(c.Config.Desktop.Environment, "sway")
	case "desktop.i3":
		return strings.EqualFold(c.Config.Desktop.Environment, "i3")
	case "editor.neovim":
		return strings.EqualFold(c.Config.Editor.Name, "neovim") || strings.EqualFold(c.Config.Editor.Name, "nvim")
	case "editor.vscode":
		return strings.EqualFold(c.Config.Editor.Name, "vscode") || strings.EqualFold(c.Config.Editor.Name, "code")
	default:
		return false
	}
}

type CheckFunc func(context.Context, Context) CheckResult
