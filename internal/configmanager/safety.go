package configmanager

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ValidateDestinationPath checks that the target destination is safe from path traversal,
// does not escape approved destination boundaries via symlinks, and resides within approved roots.
func ValidateDestinationPath(dest string, approvedRoots []string) (string, error) {
	if strings.TrimSpace(dest) == "" {
		return "", fmt.Errorf("destination path cannot be empty")
	}

	cleaned := filepath.Clean(dest)
	if !filepath.IsAbs(cleaned) {
		return "", fmt.Errorf("destination path %q must be absolute", dest)
	}

	// Check if path is within at least one approved root
	var matchedRoot string
	for _, root := range approvedRoots {
		if root == "" {
			continue
		}
		cleanRoot := filepath.Clean(root)
		if cleaned == cleanRoot || strings.HasPrefix(cleaned, cleanRoot+string(filepath.Separator)) {
			matchedRoot = cleanRoot
			break
		}
	}

	if matchedRoot == "" {
		return "", fmt.Errorf("destination path %q is outside approved destination roots (%s)", cleaned, strings.Join(approvedRoots, ", "))
	}

	// Prevent symlink-based path escape: inspect existing parent directories to ensure
	// none of them are symlinks resolving outside the approved roots.
	for parent := filepath.Dir(cleaned); parent != "" && parent != "/" && parent != "."; parent = filepath.Dir(parent) {
		info, err := os.Lstat(parent)
		if err != nil {
			if os.IsNotExist(err) {
				continue
			}
			return "", fmt.Errorf("inspect parent path %q: %w", parent, err)
		}
		if info.Mode()&os.ModeSymlink != 0 {
			resolved, err := filepath.EvalSymlinks(parent)
			if err != nil {
				return "", fmt.Errorf("resolve parent symlink %q: %w", parent, err)
			}
			// Check if resolved parent still resides within an approved root
			allowed := false
			for _, root := range approvedRoots {
				cleanRoot := filepath.Clean(root)
				if resolved == cleanRoot || strings.HasPrefix(resolved, cleanRoot+string(filepath.Separator)) {
					allowed = true
					break
				}
			}
			if !allowed {
				return "", fmt.Errorf("parent directory %q is a symlink pointing to %q outside approved roots", parent, resolved)
			}
		}
	}

	return cleaned, nil
}
