package btrfs

import (
	"path/filepath"
	"strings"
)

// CoverageReport identifies which paths are included in a prospective snapshot target.
type CoverageReport struct {
	SnapshotTarget   string   `json:"snapshot_target,omitempty"`
	CoveredPaths     []string `json:"covered_paths,omitempty"`
	ExcludedPaths    []string `json:"excluded_paths,omitempty"`
	CoverageComplete bool     `json:"coverage_complete"`
	Warnings         []string `json:"warnings,omitempty"`
}

// AnalyzeCoverage compares subvolume mount topology against a selected snapshot target.
func AnalyzeCoverage(snapshotTarget string, subvolumes []SubvolumeInfo) CoverageReport {
	report := CoverageReport{
		SnapshotTarget:   snapshotTarget,
		CoverageComplete: true,
		Warnings:         []string{},
	}
	if snapshotTarget == "" {
		snapshotTarget = "/"
	}
	if len(subvolumes) == 0 {
		report.CoveredPaths = []string{snapshotTarget}
		return report
	}

	covered := map[string]struct{}{snapshotTarget: {}}
	excluded := map[string]struct{}{}
	for _, subvolume := range subvolumes {
		if subvolume.Path == "" {
			continue
		}
		path := filepath.Clean(subvolume.Path)
		if path == "." {
			path = "/"
		}
		if path == snapshotTarget || strings.HasPrefix(path, snapshotTarget+"/") || (snapshotTarget == "/" && strings.HasPrefix(path, "/")) {
			covered[path] = struct{}{}
			continue
		}
		excluded[path] = struct{}{}
	}
	for path := range covered {
		report.CoveredPaths = append(report.CoveredPaths, path)
	}
	for path := range excluded {
		report.ExcludedPaths = append(report.ExcludedPaths, path)
	}
	if len(report.ExcludedPaths) > 0 {
		report.CoverageComplete = false
		report.Warnings = append(report.Warnings, "separate subvolumes or mountpoints were detected outside the selected snapshot target")
	}
	return report
}
