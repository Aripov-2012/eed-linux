package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/pelletier/go-toml/v2"
)

const DefaultSchemaVersion = 1

var ErrConfigExists = errors.New("configuration already exists")

type ProfileConfig struct {
	Name     string   `toml:"name"`
	Purposes []string `toml:"purposes"`
}

type DesktopConfig struct {
	Environment    string `toml:"environment"`
	DisplayManager string `toml:"display_manager"`
}

type DeveloperConfig struct {
	Git    bool `toml:"git"`
	Docker bool `toml:"docker"`
	Python bool `toml:"python"`
	NodeJS bool `toml:"nodejs"`
	Go     bool `toml:"go"`
	Rust   bool `toml:"rust"`
}

type EditorConfig struct {
	Name string `toml:"name"`
}

type AppearanceConfig struct {
	Theme string `toml:"theme"`
}

type Config struct {
	SchemaVersion int              `toml:"schema_version"`
	Profile       ProfileConfig    `toml:"profile"`
	Desktop       DesktopConfig    `toml:"desktop"`
	Developer     DeveloperConfig  `toml:"developer"`
	Editor        EditorConfig     `toml:"editor"`
	Appearance    AppearanceConfig `toml:"appearance"`
}

func DefaultConfig() Config {
	return Config{
		SchemaVersion: DefaultSchemaVersion,
		Profile: ProfileConfig{
			Name:     "developer",
			Purposes: []string{"development"},
		},
		Desktop: DesktopConfig{
			Environment:    "none",
			DisplayManager: "none",
		},
		Developer: DeveloperConfig{
			Git:    true,
			Docker: false,
			Python: true,
			NodeJS: false,
			Go:     false,
			Rust:   false,
		},
		Editor:     EditorConfig{Name: "none"},
		Appearance: AppearanceConfig{Theme: "catppuccin-mocha"},
	}
}

func LoadConfig(path string) (Config, error) {
	if path == "" {
		resolved, err := ResolveConfigPath()
		if err != nil {
			return Config{}, err
		}
		path = resolved
	}

	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			cfg := DefaultConfig()
			if err := Validate(cfg); err != nil {
				return Config{}, fmt.Errorf("default config is invalid: %w", err)
			}
			return cfg, nil
		}
		return Config{}, fmt.Errorf("read config %q: %w", path, err)
	}

	var cfg Config
	if err := toml.Unmarshal(data, &cfg); err != nil {
		return Config{}, fmt.Errorf("parse config %q: %w", path, err)
	}
	if err := Validate(cfg); err != nil {
		return Config{}, fmt.Errorf("validate config %q: %w", path, err)
	}
	return cfg, nil
}

func SaveConfig(path string, cfg Config, force bool) error {
	if err := Validate(cfg); err != nil {
		return fmt.Errorf("validate config before saving: %w", err)
	}
	if path == "" {
		resolved, err := ResolveConfigPath()
		if err != nil {
			return err
		}
		path = resolved
	}

	if _, err := os.Stat(path); err == nil && !force {
		return ErrConfigExists
	}

	if err := EnsureConfigDir(path); err != nil {
		return err
	}

	data, err := toml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	temp, err := os.CreateTemp(filepath.Dir(path), "config-*.toml")
	if err != nil {
		return fmt.Errorf("create temp config file: %w", err)
	}
	tempPath := temp.Name()
	defer os.Remove(tempPath)

	if err := temp.Chmod(0o600); err != nil {
		temp.Close()
		return fmt.Errorf("set temp config permissions: %w", err)
	}
	if _, err := temp.Write(data); err != nil {
		temp.Close()
		return fmt.Errorf("write temp config: %w", err)
	}
	if err := temp.Close(); err != nil {
		return fmt.Errorf("close temp config: %w", err)
	}

	if err := os.Rename(tempPath, path); err != nil {
		return fmt.Errorf("replace config atomically: %w", err)
	}

	return nil
}
