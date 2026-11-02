package storage

import "strings"

// DetectFilesystem identifies the filesystem type for a path using the mount table or Statfs.
func DetectFilesystem(path string) string {
	if path == "" {
		return "unknown"
	}
	for _, mount := range mustMountEntries() {
		if mount.MountPoint == path || strings.TrimSuffix(mount.MountPoint, "/") == strings.TrimSuffix(path, "/") {
			if mount.FSType != "" {
				return mount.FSType
			}
		}
	}
	return detectFilesystem(path)
}

func mustMountEntries() []MountEntry {
	entries, err := ParseMountInfoFile("/proc/self/mountinfo")
	if err != nil {
		return nil
	}
	return entries
}
