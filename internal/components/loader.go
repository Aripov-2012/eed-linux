package components

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/pelletier/go-toml/v2"
)

// LoadFromDir reads all TOML manifests from a directory and returns a registry.
func LoadFromDir(dir string) (*Registry, error) {
	if dir == "" {
		return nil, fmt.Errorf("manifest directory is empty")
	}
	return LoadFromFS(os.DirFS(dir), ".")
}

// LoadFromFS reads all TOML manifests from a filesystem directory and returns a registry.
func LoadFromFS(filesystem fs.FS, dir string) (*Registry, error) {
	if filesystem == nil {
		return nil, fmt.Errorf("manifest filesystem is nil")
	}
	if dir == "" {
		return nil, fmt.Errorf("manifest directory is empty")
	}
	entries, err := fs.ReadDir(filesystem, dir)
	if err != nil {
		return nil, fmt.Errorf("read manifest directory %q: %w", dir, err)
	}
	registry := NewRegistry()
	paths := make([]string, 0, len(entries))
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		if filepath.Ext(entry.Name()) == ".toml" {
			paths = append(paths, entry.Name())
		}
	}
	sort.Strings(paths)
	for _, path := range paths {
		manifestPath := path
		if dir != "." {
			manifestPath = filepath.ToSlash(filepath.Join(dir, path))
		}
		data, err := fs.ReadFile(filesystem, manifestPath)
		if err != nil {
			return nil, fmt.Errorf("read manifest %q: %w", manifestPath, err)
		}
		var manifest Manifest
		if err := toml.Unmarshal(data, &manifest); err != nil {
			return nil, fmt.Errorf("parse manifest %q: %w", manifestPath, err)
		}
		manifest.ID = normalizeManifestID(manifest.ID)
		manifest.Name = strings.TrimSpace(manifest.Name)
		manifest.Category = strings.TrimSpace(manifest.Category)
		manifest.Description = strings.TrimSpace(manifest.Description)
		manifest.LoadedAt = time.Now().UTC()
		if err := registry.Add(manifest); err != nil {
			return nil, fmt.Errorf("register manifest %q: %w", manifestPath, err)
		}
	}
	if err := registry.ValidateDependencies(); err != nil {
		return nil, err
	}
	if err := registry.DetectCycles(); err != nil {
		return nil, err
	}
	return registry, nil
}

func normalizeManifestID(value string) string {
	return strings.TrimSpace(strings.ToLower(value))
}
