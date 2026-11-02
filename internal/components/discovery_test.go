package components

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	builtin "eed"
)

func TestDiscoverManifestSourceFromRepositoryRootAndDist(t *testing.T) {
	root, err := filepath.Abs(filepath.Join("..", ".."))
	if err != nil {
		t.Fatal(err)
	}
	for _, workingDirectory := range []string{root, filepath.Join(root, "dist")} {
		t.Run(workingDirectory, func(t *testing.T) {
			source, err := DiscoverManifestSource(DiscoveryOptions{
				WorkingDir:    workingDirectory,
				Executable:    filepath.Join(root, "dist", "eed"),
				SystemDataDir: filepath.Join(t.TempDir(), "missing"),
				Builtin:       builtin.BuiltinManifests(),
			})
			if err != nil {
				t.Fatal(err)
			}
			if source.Location != "embedded built-in manifests" {
				t.Fatalf("embedded manifests have higher priority than development roots, got %q", source.Location)
			}
		})
	}
}

func TestEmbeddedManifestSourceLoadsBuiltInRegistry(t *testing.T) {
	source, err := DiscoverManifestSource(DiscoveryOptions{Builtin: builtin.BuiltinManifests()})
	if err != nil {
		t.Fatal(err)
	}
	registry, err := LoadFromFS(source.Filesystem, source.Directory)
	if err != nil {
		t.Fatal(err)
	}
	if !registry.Has("desktop.hyprland") || len(registry.Order) == 0 {
		t.Fatalf("embedded manifests should load as the built-in registry: %v", registry.Order)
	}
}

func TestDiscoverManifestSourceUsesDevelopmentRootWithoutEmbeddedData(t *testing.T) {
	root := t.TempDir()
	writeProjectMarker(t, root)
	for _, workingDirectory := range []string{root, filepath.Join(root, "dist")} {
		if err := os.MkdirAll(workingDirectory, 0o755); err != nil {
			t.Fatal(err)
		}
		source, err := DiscoverManifestSource(DiscoveryOptions{
			WorkingDir:    workingDirectory,
			Executable:    filepath.Join(root, "dist", "eed"),
			SystemDataDir: filepath.Join(t.TempDir(), "missing"),
		})
		if err != nil {
			t.Fatal(err)
		}
		if source.Location != root || source.Directory != "components/manifests" {
			t.Fatalf("expected project root %q, got %+v", root, source)
		}
	}
}

func TestDiscoverManifestSourceResolvesAbsoluteExecutableSymlink(t *testing.T) {
	root := t.TempDir()
	writeProjectMarker(t, root)
	binDir := filepath.Join(root, "dist")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	target := filepath.Join(root, "eed")
	if err := os.WriteFile(target, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	link := filepath.Join(binDir, "eed")
	if err := os.Symlink(target, link); err != nil {
		t.Fatal(err)
	}
	source, err := DiscoverManifestSource(DiscoveryOptions{
		WorkingDir: t.TempDir(), Executable: link,
		SystemDataDir: filepath.Join(t.TempDir(), "missing"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if source.Location != root {
		t.Fatalf("expected symlink target project root %q, got %q", root, source.Location)
	}
}

func TestDiscoverManifestSourceWorksFromUnrelatedDirectoryWithAbsoluteExecutablePath(t *testing.T) {
	root := t.TempDir()
	writeProjectMarker(t, root)
	binDir := filepath.Join(root, "dist")
	if err := os.MkdirAll(binDir, 0o755); err != nil {
		t.Fatal(err)
	}
	executable := filepath.Join(binDir, "eed")
	if err := os.WriteFile(executable, []byte("binary"), 0o755); err != nil {
		t.Fatal(err)
	}
	unrelatedDir := t.TempDir()
	source, err := DiscoverManifestSource(DiscoveryOptions{
		WorkingDir:    unrelatedDir,
		Executable:    executable,
		SystemDataDir: filepath.Join(t.TempDir(), "missing"),
	})
	if err != nil {
		t.Fatal(err)
	}
	if source.Location != root {
		t.Fatalf("expected executable location root %q, got %q", root, source.Location)
	}
}

func TestDiscoverManifestSourcePrefersExplicitAndEnvironmentDataDirs(t *testing.T) {
	explicit := makeManifestDirectory(t, "explicit")
	environment := makeManifestDirectory(t, "environment")
	source, err := DiscoverManifestSource(DiscoveryOptions{
		DataDir: explicit, Environment: environment, Builtin: builtin.BuiltinManifests(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if source.Location != explicit {
		t.Fatalf("explicit data directory must win, got %q", source.Location)
	}
	source, err = DiscoverManifestSource(DiscoveryOptions{
		Environment: environment, Builtin: builtin.BuiltinManifests(),
	})
	if err != nil {
		t.Fatal(err)
	}
	if source.Location != environment {
		t.Fatalf("environment directory must win over embedded data, got %q", source.Location)
	}
}

func TestDiscoverManifestSourceFindsSystemResourcesFromUnrelatedDirectory(t *testing.T) {
	systemDir := makeManifestDirectory(t, "system")
	source, err := DiscoverManifestSource(DiscoveryOptions{
		WorkingDir: t.TempDir(), Executable: filepath.Join(t.TempDir(), "eed"),
		SystemDataDir: systemDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if source.Location != systemDir {
		t.Fatalf("expected system resources %q, got %q", systemDir, source.Location)
	}
}

func TestDiscoverManifestSourcePrefersSystemResourcesOverDevelopmentRoot(t *testing.T) {
	root := t.TempDir()
	writeProjectMarker(t, root)
	systemDir := makeManifestDirectory(t, "system")
	source, err := DiscoverManifestSource(DiscoveryOptions{
		WorkingDir: root, SystemDataDir: systemDir,
	})
	if err != nil {
		t.Fatal(err)
	}
	if source.Location != systemDir {
		t.Fatalf("system resource directory must be preferred to development discovery, got %+v", source)
	}
}

func TestDiscoverManifestSourceReturnsActionableErrorForInvalidConfiguredDirectory(t *testing.T) {
	_, err := DiscoverManifestSource(DiscoveryOptions{
		DataDir: filepath.Join(t.TempDir(), "missing"), Builtin: builtin.BuiltinManifests(),
	})
	if err == nil || !containsAny(err.Error(), "--data-dir", "EED_DATA_DIR") {
		t.Fatalf("expected actionable configured-directory error, got %v", err)
	}
}

func makeManifestDirectory(t *testing.T, name string) string {
	t.Helper()
	root := filepath.Join(t.TempDir(), name)
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "sample.toml"), []byte("schema_version=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	return root
}

func writeProjectMarker(t *testing.T, root string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(root, "components", "manifests"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "go.mod"), []byte("module example\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "components", "manifests", "sample.toml"), []byte("schema_version=1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
}

func containsAny(value string, substrings ...string) bool {
	for _, substring := range substrings {
		if strings.Contains(value, substring) {
			return true
		}
	}
	return false
}
