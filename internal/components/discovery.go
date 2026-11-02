package components

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

const systemDataDir = "/usr/share/eed"

type DiscoveryOptions struct {
	DataDir       string
	Environment   string
	WorkingDir    string
	Executable    string
	SystemDataDir string
	Builtin       fs.FS
}

type ManifestSource struct {
	Filesystem fs.FS
	Directory  string
	Location   string
}

func DiscoverManifestSource(options DiscoveryOptions) (ManifestSource, error) {
	if options.DataDir == "" {
		options.DataDir = strings.TrimSpace(options.Environment)
	}
	if options.DataDir != "" {
		source, err := sourceFromDirectory(options.DataDir)
		if err != nil {
			return ManifestSource{}, fmt.Errorf("configured component data directory %q is invalid (%w); check --data-dir or EED_DATA_DIR", options.DataDir, err)
		}
		source.Location = options.DataDir
		return source, nil
	}

	if options.Builtin != nil {
		if directory, ok := firstManifestDirectory(options.Builtin, []string{"components/manifests", "."}); ok {
			return ManifestSource{Filesystem: options.Builtin, Directory: directory, Location: "embedded built-in manifests"}, nil
		}
	}

	systemDir := options.SystemDataDir
	if systemDir == "" {
		systemDir = systemDataDir
	}
	if source, err := sourceFromDirectory(systemDir); err == nil {
		source.Location = systemDir
		return source, nil
	}

	roots := make([]string, 0, 2)
	if options.WorkingDir != "" {
		roots = append(roots, options.WorkingDir)
	}
	if options.Executable != "" {
		executable, err := filepath.Abs(options.Executable)
		if err == nil {
			if resolved, resolveErr := filepath.EvalSymlinks(executable); resolveErr == nil {
				executable = resolved
			}
			roots = append(roots, filepath.Dir(executable))
		}
	}
	seen := map[string]bool{}
	for _, root := range roots {
		for current := filepath.Clean(root); ; current = filepath.Dir(current) {
			if seen[current] {
				break
			}
			seen[current] = true
			if isDevelopmentRoot(current) {
				return ManifestSource{
					Filesystem: os.DirFS(current), Directory: "components/manifests", Location: current,
				}, nil
			}
			parent := filepath.Dir(current)
			if parent == current {
				break
			}
		}
	}
	return ManifestSource{}, fmt.Errorf("component manifests were not found; pass --data-dir, set EED_DATA_DIR, or install resources under %s", systemDir)
}

func LoadDiscovered(dataDir string, builtin fs.FS) (*Registry, error) {
	source, err := DiscoverManifestSource(DiscoveryOptions{
		DataDir: dataDir, Environment: os.Getenv("EED_DATA_DIR"),
		WorkingDir: currentWorkingDirectory(), Executable: currentExecutable(),
		Builtin: builtin,
	})
	if err != nil {
		return nil, err
	}
	return LoadFromFS(source.Filesystem, source.Directory)
}

func sourceFromDirectory(root string) (ManifestSource, error) {
	if strings.TrimSpace(root) == "" {
		return ManifestSource{}, fmt.Errorf("directory is empty")
	}
	candidates := []string{"components/manifests", "manifests", "components", "."}
	if directory, ok := firstManifestDirectory(os.DirFS(root), candidates); ok {
		return ManifestSource{Filesystem: os.DirFS(root), Directory: directory}, nil
	}
	return ManifestSource{}, fmt.Errorf("no TOML component manifests found directly or under components/manifests")
}

func firstManifestDirectory(filesystem fs.FS, candidates []string) (string, bool) {
	for _, candidate := range candidates {
		entries, err := fs.ReadDir(filesystem, candidate)
		if err != nil {
			continue
		}
		for _, entry := range entries {
			if !entry.IsDir() && filepath.Ext(entry.Name()) == ".toml" {
				return candidate, true
			}
		}
	}
	return "", false
}

func isDevelopmentRoot(root string) bool {
	if _, err := os.Stat(filepath.Join(root, "go.mod")); err != nil {
		return false
	}
	_, ok := firstManifestDirectory(os.DirFS(root), []string{"components/manifests"})
	return ok
}

func currentWorkingDirectory() string {
	directory, _ := os.Getwd()
	return directory
}

func currentExecutable() string {
	executable, _ := os.Executable()
	return executable
}
