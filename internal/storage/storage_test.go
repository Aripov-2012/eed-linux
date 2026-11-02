package storage

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"eed/internal/storage/btrfs"
)

func TestParseMountInfo(t *testing.T) {
	data := `31 28 0:22 / / rw,relatime - btrfs /dev/sda1 rw,space_cache
32 28 0:23 / /home rw,relatime - ext4 /dev/sdb1 rw`
	entries, err := ParseMountInfo(data)
	if err != nil {
		t.Fatalf("ParseMountInfo returned error: %v", err)
	}
	if len(entries) != 2 {
		t.Fatalf("expected 2 mount entries, got %d", len(entries))
	}
	if entries[0].MountPoint != "/" {
		t.Fatalf("expected root mount point, got %q", entries[0].MountPoint)
	}
	if entries[1].MountPoint != "/home" {
		t.Fatalf("expected /home mount point, got %q", entries[1].MountPoint)
	}
}

func TestParseMountInfoEscapedPath(t *testing.T) {
	data := `40 28 0:40 / /tmp\040dir rw,relatime - xfs /dev/sdc1 rw`
	entries, err := ParseMountInfo(data)
	if err != nil {
		t.Fatalf("ParseMountInfo returned error: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 mount entry, got %d", len(entries))
	}
	if entries[0].MountPoint != "/tmp dir" {
		t.Fatalf("expected escaped mount point to be unescaped, got %q", entries[0].MountPoint)
	}
}

func TestAnalyzeReadsBasicCapacityWithoutRootAndSeparatesFreeFromAvailable(t *testing.T) {
	report, err := Analyze()
	if err != nil {
		t.Fatalf("read-only storage analysis failed for an unprivileged user: %v", err)
	}
	var root *Filesystem
	for i := range report.Filesystems {
		if report.Filesystems[i].MountPoint == "/" {
			root = &report.Filesystems[i]
			break
		}
	}
	if root == nil {
		t.Fatalf("expected root filesystem capacity in report: %+v", report.Filesystems)
	}
	if root.TotalBytes <= 0 || root.AvailableBytes <= 0 || root.FreeBytes <= 0 {
		t.Fatalf("expected readable capacity, free blocks and available blocks: %+v", *root)
	}
	if root.FreeBytes < root.AvailableBytes {
		t.Fatalf("available-to-user blocks must not exceed all free blocks: %+v", *root)
	}
}

func TestExt4CapacityMetricsArePreserved(t *testing.T) {
	mountInfoPath := filepath.Join(t.TempDir(), "mountinfo")
	if err := os.WriteFile(mountInfoPath, []byte(`31 28 8:1 / / rw,relatime - ext4 /dev/sda1 rw`), 0o600); err != nil {
		t.Fatal(err)
	}
	report, err := analyze(mountInfoPath, func(string) (fsStats, error) {
		return fsStats{TotalBytes: 1000, UsedBytes: 400, FreeBytes: 600, AvailableBytes: 500}, nil
	}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if report.Summary.RootFilesystem != "ext4" || len(report.Filesystems) != 1 {
		t.Fatalf("expected ext4 root to be identified: %+v", report)
	}
	fs := report.Filesystems[0]
	if fs.TotalBytes != 1000 || fs.UsedBytes != 400 || fs.FreeBytes != 600 || fs.AvailableBytes != 500 {
		t.Fatalf("expected all filesystem capacity metrics to be preserved: %+v", fs)
	}
}

type lvmFixtureRunner struct {
	output string
	args   []string
	err    error
}

func (r *lvmFixtureRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	r.args = append([]string{name}, args...)
	return []byte(r.output), r.err
}

func TestLVMThinVolumeIsInspectedReadOnlyWithMockedRunner(t *testing.T) {
	report := Report{Filesystems: []Filesystem{{
		MountPoint: "/", SourceDevice: "/dev/mapper/vg-root", Filesystem: "ext4",
	}}}
	runner := &lvmFixtureRunner{output: "Vwi-a-tz--"}
	EnrichWithLVMUsing(&report, runner, func(name string) (string, error) {
		if name != "lvs" {
			t.Fatalf("unexpected executable lookup: %s", name)
		}
		return "/usr/bin/lvs", nil
	})
	root := report.Filesystems[0]
	if !root.LVMLogicalVolume || !root.LVMThin || root.LVMStatus != "available" {
		t.Fatalf("expected thin LVM volume classification: %+v", root)
	}
	if len(runner.args) != 6 || runner.args[0] != "/usr/bin/lvs" ||
		runner.args[1] != "--readonly" || runner.args[5] != "/dev/mapper/vg-root" {
		t.Fatalf("LVM probing must be read-only and target the detected root volume: %#v", runner.args)
	}
}

var _ btrfs.CommandRunner = (*btrfsFixtureRunner)(nil)

type btrfsFixtureRunner struct {
	calls [][]string
}

func (r *btrfsFixtureRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	r.calls = append(r.calls, append([]string{name}, args...))
	if len(args) >= 2 && args[0] == "subvolume" && args[1] == "list" {
		return []byte("ID 256 gen 10 top level 5 path @root\nID 257 gen 10 top level 5 path @home\n"), nil
	}
	if len(args) >= 2 && args[0] == "filesystem" && args[1] == "usage" {
		return []byte("Device size: 100GiB\nDevice allocated: 40GiB\nDevice unallocated: 60GiB\n"), nil
	}
	return nil, errors.New("unsupported command")
}

func TestBtrfsSeparateMountsAndSubvolumesInspectedReadOnly(t *testing.T) {
	report := Report{Filesystems: []Filesystem{
		{MountPoint: "/", SourceDevice: "/dev/sda1", Filesystem: "btrfs"},
		{MountPoint: "/home", SourceDevice: "/dev/sda1", Filesystem: "btrfs"},
		{MountPoint: "/var", SourceDevice: "/dev/sda1", Filesystem: "btrfs"},
	}}
	runner := &btrfsFixtureRunner{}
	EnrichWithBtrfsUsageUsing(&report, runner, func(name string) (string, error) {
		if name != "btrfs" {
			t.Fatalf("unexpected executable lookup: %s", name)
		}
		return "/usr/bin/btrfs", nil
	})
	if !report.BtrfsLayoutInspected || len(report.BtrfsSubvolumes) != 2 {
		t.Fatalf("expected Btrfs subvolumes to be parsed: %+v", report)
	}
	if len(report.BtrfsSeparateMounts) != 2 || report.BtrfsSeparateMounts[0] != "/home" || report.BtrfsSeparateMounts[1] != "/var" {
		t.Fatalf("expected separate Btrfs mounts /home and /var: %+v", report.BtrfsSeparateMounts)
	}
	if report.BtrfsUsage == nil || report.BtrfsUsage.DeviceSizeBytes == 0 {
		t.Fatalf("expected Btrfs usage to be enriched: %+v", report.BtrfsUsage)
	}
}

func TestAnalyzePreservesCapacityWhenOptionalInspectionFails(t *testing.T) {
	mountInfoPath := filepath.Join(t.TempDir(), "mountinfo")
	mountInfo := `31 28 8:1 / / rw,relatime - btrfs /dev/sda1 rw
32 28 8:2 / /restricted rw,relatime - ext4 /dev/sdb1 rw
33 28 0:44 / /run/user/1000/doc rw,nosuid,nodev - fuse.portal portal rw
34 28 0:45 / /var/lib/docker/overlay2/abc/merged rw,relatime - overlay overlay rw
`
	if err := os.WriteFile(mountInfoPath, []byte(mountInfo), 0o600); err != nil {
		t.Fatal(err)
	}
	statFilesystem := func(path string) (fsStats, error) {
		if path == "/restricted" {
			return fsStats{}, os.ErrPermission
		}
		if path == "/run/user/1000/doc" || strings.Contains(path, "/overlay2/") {
			t.Errorf("pseudo-filesystem and container overlay should not be stat'ed: %s", path)
		}
		return fsStats{TotalBytes: 150, UsedBytes: 50, FreeBytes: 100, AvailableBytes: 70}, nil
	}
	report, err := analyze(mountInfoPath, statFilesystem, func(report *Report) {
		report.Warnings = append(report.Warnings, "optional Btrfs usage inspection unavailable at /: operation not permitted")
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(report.Filesystems) != 1 {
		t.Fatalf("expected accessible root filesystem to survive optional failures: %+v", report)
	}
	root := report.Filesystems[0]
	if root.MountPoint != "/" || root.TotalBytes != 150 || root.FreeBytes != 100 || root.AvailableBytes != 70 {
		t.Fatalf("expected distinct root capacity/free/available values: %+v", root)
	}
	warnings := strings.Join(report.Warnings, "\n")
	if !strings.Contains(warnings, "skipped 1 inaccessible optional mountpoints (/restricted): permission denied") ||
		!strings.Contains(warnings, "optional Btrfs usage inspection unavailable") {
		t.Fatalf("expected contextual optional inspection warnings: %v", report.Warnings)
	}
	if len(report.SkippedMounts) != 3 ||
		report.SkippedMounts[0].Category != "pseudo_filesystem" ||
		report.SkippedMounts[1].Category != "container_overlay" ||
		report.SkippedMounts[2].Category != "inaccessible_optional" {
		t.Fatalf("expected grouped pseudo, overlay, and inaccessible mounts: %+v", report.SkippedMounts)
	}
}

func TestAnalyzeReturnsCauseOnlyWhenNoFilesystemMetricsCanBeRead(t *testing.T) {
	mountInfoPath := filepath.Join(t.TempDir(), "mountinfo")
	if err := os.WriteFile(mountInfoPath, []byte(`31 28 8:1 / / rw,relatime - ext4 /dev/sda1 rw`), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := analyze(mountInfoPath, func(string) (fsStats, error) {
		return fsStats{}, errors.New("operation not permitted")
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "unable to determine storage information") ||
		!strings.Contains(err.Error(), "operation not permitted") {
		t.Fatalf("expected explicit failure only when no metrics can be retrieved: %v", err)
	}
}

func TestInaccessibleMountWarningAggregatesDistinctFailures(t *testing.T) {
	warning := inaccessibleMountWarning([]SkippedMountGroup{{
		Category:    "inaccessible_optional",
		Count:       4,
		MountPoints: []string{"/run/a", "/run/b", "/run/c"},
		Reasons:     []string{"permission denied", "operation not permitted"},
	}})
	if !strings.Contains(warning, "skipped 4 inaccessible optional mountpoints") ||
		!strings.Contains(warning, "permission denied") {
		t.Fatalf("expected optional mount failures to be summarized once: %q", warning)
	}
}

func TestAnalyzeDoesNotHideTargetFilesystemFailure(t *testing.T) {
	mountInfoPath := filepath.Join(t.TempDir(), "mountinfo")
	mountInfo := `31 28 8:1 / / rw,relatime - ext4 /dev/sda1 rw
32 28 8:2 / /home rw,relatime - ext4 /dev/sdb1 rw
`
	if err := os.WriteFile(mountInfoPath, []byte(mountInfo), 0o600); err != nil {
		t.Fatal(err)
	}
	report, err := analyze(mountInfoPath, func(path string) (fsStats, error) {
		if path == "/" {
			return fsStats{}, os.ErrPermission
		}
		return fsStats{TotalBytes: 250, FreeBytes: 150, AvailableBytes: 125}, nil
	}, nil)
	if err == nil || !strings.Contains(err.Error(), "required target filesystem capacity unavailable at /") {
		t.Fatalf("expected target filesystem failure to remain explicit, got %v", err)
	}
	if len(report.Filesystems) != 1 || report.Filesystems[0].MountPoint != "/home" {
		t.Fatalf("expected available optional capacity to remain in report: %+v", report.Filesystems)
	}
}
