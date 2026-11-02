package components

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadFromDir(t *testing.T) {
	dir := filepath.Join("..", "..", "components", "manifests")
	registry, err := LoadFromDir(dir)
	if err != nil {
		t.Fatalf("load registry: %v", err)
	}
	if !registry.Has("desktop.hyprland") {
		t.Fatal("expected hyprland manifest to be present")
	}
}

func TestEveryManifestDocumentsItsSizeEstimateAvailability(t *testing.T) {
	registry, err := LoadFromDir(filepath.Join("..", "..", "components", "manifests"))
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range registry.Order {
		manifest, ok := registry.Get(id)
		if !ok {
			t.Fatalf("manifest %q missing from registry", id)
		}
		if manifest.Resources.EstimateConfidence == "" || manifest.Resources.EstimateNotes == "" {
			t.Errorf("manifest %q must explain the confidence and availability of its size estimate", id)
		}
	}
}

func TestDuplicateComponentIDs(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, "dup-a.toml", `
schema_version = 1
id = "desktop.demo"
name = "Demo"
category = "desktop"
supported_os = ["arch"]
supported_architectures = ["x86_64"]
packages = ["demo"]
`)
	writeManifest(t, dir, "dup-b.toml", `
schema_version = 1
id = "desktop.demo"
name = "Demo 2"
category = "desktop"
supported_os = ["arch"]
supported_architectures = ["x86_64"]
packages = ["demo2"]
`)
	_, err := LoadFromDir(dir)
	if err == nil {
		t.Fatal("expected duplicate ID error")
	}
}

func TestMissingDependency(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, "missing-dep.toml", `
schema_version = 1
id = "desktop.missing"
name = "Missing Dep"
category = "desktop"
supported_os = ["arch"]
supported_architectures = ["x86_64"]
dependencies = ["system.ghost"]
packages = ["ghost"]
`)
	_, err := LoadFromDir(dir)
	if err == nil {
		t.Fatal("expected missing dependency error")
	}
}

func TestDependencyCycle(t *testing.T) {
	dir := t.TempDir()
	writeManifest(t, dir, "a.toml", `
schema_version = 1
id = "a"
name = "A"
category = "system"
supported_os = ["arch"]
supported_architectures = ["x86_64"]
dependencies = ["b"]
packages = ["a"]
`)
	writeManifest(t, dir, "b.toml", `
schema_version = 1
id = "b"
name = "B"
category = "system"
supported_os = ["arch"]
supported_architectures = ["x86_64"]
dependencies = ["a"]
packages = ["b"]
`)
	_, err := LoadFromDir(dir)
	if err == nil {
		t.Fatal("expected cycle detection error")
	}
}

func writeManifest(t *testing.T, dir, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(content), 0o600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
}
