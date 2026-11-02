package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestResolveConfigPathUsesXDGHome(t *testing.T) {
	t.Setenv("XDG_CONFIG_HOME", "/tmp/eed-test")
	path, err := ResolveConfigPath()
	if err != nil {
		t.Fatalf("ResolveConfigPath returned error: %v", err)
	}
	if path != filepath.Join("/tmp/eed-test", "eed", "config.toml") {
		t.Fatalf("unexpected path: %s", path)
	}
}

func TestDefaultConfigIsValid(t *testing.T) {
	cfg := DefaultConfig()
	if err := Validate(cfg); err != nil {
		t.Fatalf("default config invalid: %v", err)
	}
}

func TestLoadConfigReturnsDefaultWhenMissing(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	cfg, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig returned error: %v", err)
	}
	if cfg.SchemaVersion != DefaultSchemaVersion {
		t.Fatalf("invalid schema version: %d", cfg.SchemaVersion)
	}
}

func TestSaveConfigCreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.toml")

	cfg := DefaultConfig()
	if err := SaveConfig(path, cfg, true); err != nil {
		t.Fatalf("SaveConfig returned error: %v", err)
	}

	if _, err := os.Stat(path); err != nil {
		t.Fatalf("config file not created: %v", err)
	}
}

func TestValidationRejectsBadConfig(t *testing.T) {
	cfg := DefaultConfig()
	cfg.Desktop.Environment = "not-a-desktop"
	if err := Validate(cfg); err == nil {
		t.Fatal("expected validation failure")
	}
}
