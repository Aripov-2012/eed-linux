package system

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"syscall"

	"golang.org/x/sys/unix"
)

func detectStorage(opts DetectOptions) ([]MountInfo, []string, error) {
	items := []MountInfo{}
	mounts := []MountInfo{}
	warnings := []string{}
	mountInfoPath := opts.MountInfoPath
	if mountInfoPath == "" {
		mountInfoPath = filepath.Join(opts.ProcRoot, "self", "mountinfo")
	}
	if data, err := os.ReadFile(mountInfoPath); err == nil {
		for _, line := range strings.Split(string(data), "\n") {
			entry, ok := parseMountEntry(line)
			if !ok {
				continue
			}
			entry.Category = mountCategory(entry)
			mounts = append(mounts, entry)
		}
	} else {
		warnings = append(warnings, fmt.Sprintf("read mount information %s: %v", mountInfoPath, err))
	}
	rootMajor, rootMinor, haveRootDevice := 0, 0, false
	for _, mount := range mounts {
		if filepath.Clean(mount.MountPoint) == "/" {
			rootMajor, rootMinor, haveRootDevice = mount.Major, mount.Minor, true
			break
		}
	}
	for index := range mounts {
		mount := &mounts[index]
		if filepath.Clean(mount.MountPoint) == "/home" && haveRootDevice &&
			!strings.EqualFold(mount.Filesystem, "btrfs") &&
			mount.Major == rootMajor && mount.Minor == rootMinor &&
			filepath.Clean(mount.Root) != "/" {
			mount.BindMount = true
			mount.Category = "bind"
		}
	}
	hasRoot := false
	for _, mount := range mounts {
		if filepath.Clean(mount.MountPoint) == "/" {
			hasRoot = true
			break
		}
	}
	if !hasRoot {
		mounts = append(mounts, MountInfo{MountPoint: "/", SourceDevice: "unknown", Category: "root"})
	}
	statPaths := map[string]fsStats{}
	statErrors := map[string]error{}
	var rootErr error
	stat := opts.StatFS
	if stat == nil {
		stat = statFS
	}
	for _, mount := range mounts {
		root := mount.MountPoint
		if root == "" {
			continue
		}
		info := fsStats{}
		var err error
		if statErrors[root] != nil {
			err = statErrors[root]
		} else if cached, ok := statPaths[root]; ok {
			info = cached
		} else if categoryNeedsCapacity(mount.Category) {
			info, err = stat(root)
			if err != nil {
				statErrors[root] = err
			} else {
				statPaths[root] = info
			}
		}
		if err != nil {
			if filepath.Clean(root) == "/" {
				rootErr = fmt.Errorf("required root filesystem is inaccessible: %w", err)
			}
			items = append(items, mount)
			continue
		}
		mount.TotalBytes = info.TotalBytes
		mount.UsedBytes = info.UsedBytes
		mount.FreeBytes = info.FreeBytes
		mount.AvailableBytes = info.AvailableBytes
		mount.CapacityKnown = categoryNeedsCapacity(mount.Category)
		items = append(items, mount)
	}
	if rootErr != nil {
		return items, warnings, rootErr
	}
	if len(statErrors) > 0 {
		paths := make([]string, 0, len(statErrors))
		for path := range statErrors {
			paths = append(paths, path)
		}
		sort.Strings(paths)
		sample := paths
		if len(sample) > 3 {
			sample = sample[:3]
		}
		warnings = append(warnings, fmt.Sprintf("%d optional mountpoint(s) are inaccessible: %s", len(paths), strings.Join(sample, ", ")))
	}
	if len(items) == 0 {
		return nil, warnings, fmt.Errorf("unable to determine storage information")
	}
	return items, warnings, nil
}

func parseMountEntry(line string) (MountInfo, bool) {
	fields := strings.Fields(line)
	if len(fields) < 10 {
		return MountInfo{}, false
	}
	separator := -1
	for index, field := range fields {
		if field == "-" {
			separator = index
			break
		}
	}
	if separator < 6 || separator+2 >= len(fields) {
		return MountInfo{}, false
	}
	majorMinor := strings.SplitN(fields[2], ":", 2)
	if len(majorMinor) != 2 {
		return MountInfo{}, false
	}
	major, majorErr := strconv.Atoi(majorMinor[0])
	minor, minorErr := strconv.Atoi(majorMinor[1])
	if majorErr != nil || minorErr != nil {
		return MountInfo{}, false
	}
	return MountInfo{
		Root:       unescapeMountField(fields[3]),
		MountPoint: unescapeMountField(fields[4]), SourceDevice: unescapeMountField(fields[separator+2]),
		Filesystem: fields[separator+1], Major: major, Minor: minor,
		ReadOnly: strings.Contains(","+fields[5]+",", ",ro,"),
	}, true
}

func unescapeMountField(value string) string {
	replacer := strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`)
	return replacer.Replace(value)
}

func isPseudoFilesystem(name string) bool {
	switch strings.ToLower(name) {
	case "autofs", "binfmt_misc", "bpf", "cgroup", "cgroup2", "configfs", "debugfs", "devpts", "devtmpfs",
		"efivarfs", "fusectl", "hugetlbfs", "mqueue", "nsfs", "proc", "pstore", "ramfs",
		"rpc_pipefs", "securityfs", "sysfs", "tmpfs", "tracefs":
		return true
	default:
		return false
	}
}

func mountCategory(mount MountInfo) string {
	fs := strings.ToLower(mount.Filesystem)
	point := filepath.Clean(mount.MountPoint)
	switch {
	case point == "/":
		return "root"
	case point == "/boot/efi" || point == "/efi":
		return "efi"
	case point == "/boot" || strings.HasPrefix(point, "/boot/"):
		return "boot"
	case fs == "squashfs" && (strings.HasPrefix(point, "/snap/") || strings.HasPrefix(mount.SourceDevice, "/dev/loop")):
		return "snap"
	case fs == "overlay":
		return "overlay"
	case isRemoteFilesystem(fs):
		return "remote"
	case fs == "squashfs" || fs == "erofs":
		return "virtual"
	case fs == "nsfs" || strings.Contains(point, "/ns/"):
		return "namespace"
	case isPseudoFilesystem(fs):
		return "pseudo"
	case strings.HasPrefix(fs, "fuse"):
		return "fuse"
	case point == "/home":
		return "home"
	default:
		return "local"
	}
}

func isRemoteFilesystem(name string) bool {
	switch name {
	case "9p", "ceph", "cifs", "glusterfs", "nfs", "nfs4", "smb3":
		return true
	default:
		return false
	}
}

func categoryNeedsCapacity(category string) bool {
	switch category {
	case "snap", "pseudo", "overlay", "namespace", "virtual":
		return false
	default:
		return true
	}
}

func detectBlockDevices(opts DetectOptions) ([]BlockDevice, error) {
	root := filepath.Join(opts.SysfsRoot, "class", "block")
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, fmt.Errorf("read block device inventory %s: %w", root, err)
	}
	result := make([]BlockDevice, 0, len(entries))
	for _, entry := range entries {
		name := entry.Name()
		base := filepath.Join(root, name)
		sectors, err := readInt(filepath.Join(base, "size"))
		device := BlockDevice{
			Name: name, Path: filepath.Join("/dev", name), SizeBytes: sectors * 512, SizeKnown: err == nil,
		}
		resolved, resolveErr := filepath.EvalSymlinks(base)
		if resolveErr != nil {
			resolved = base
		}
		if _, err := os.Stat(filepath.Join(base, "partition")); err == nil {
			device.IsPartition = true
			device.Kind = "partition"
			device.Parent = partitionParent(name)
			if resolvedParent := filepath.Base(filepath.Dir(resolved)); resolvedParent != "block" && resolvedParent != filepath.Base(root) {
				device.Parent = resolvedParent
			}
		} else {
			device.Kind = classifyBlockDevice(name, resolved)
		}
		device.Transport = blockTransport(name, resolved)
		if data, err := os.ReadFile(filepath.Join(base, "dev")); err == nil {
			parts := strings.SplitN(strings.TrimSpace(string(data)), ":", 2)
			if len(parts) == 2 {
				device.Major, _ = strconv.Atoi(parts[0])
				device.Minor, _ = strconv.Atoi(parts[1])
			}
		}
		result = append(result, device)
	}
	return result, nil
}

func classifyBlockDevice(name, sysfsPath string) string {
	switch {
	case strings.HasPrefix(name, "loop"):
		return "loop"
	case strings.HasPrefix(name, "dm-"):
		return "device_mapper"
	case strings.HasPrefix(name, "ram") || strings.HasPrefix(name, "zram"):
		return "virtual"
	case strings.HasPrefix(name, "vd") || strings.HasPrefix(name, "xvd") ||
		strings.Contains(sysfsPath, "/devices/virtual/") ||
		strings.Contains(sysfsPath, "/virtio") || strings.Contains(sysfsPath, "/xen"):
		return "virtual"
	default:
		return "physical_disk"
	}
}

func blockTransport(name, sysfsPath string) string {
	switch {
	case strings.HasPrefix(name, "nvme"):
		return "nvme"
	case strings.Contains(sysfsPath, "/usb"):
		return "usb"
	case strings.Contains(sysfsPath, "/sas"):
		return "sas"
	case strings.HasPrefix(name, "vd") || strings.Contains(sysfsPath, "/virtio"):
		return "virtio"
	case strings.HasPrefix(name, "xvd") || strings.Contains(sysfsPath, "/xen"):
		return "xen"
	case strings.Contains(sysfsPath, "/ata"):
		return "sata"
	case strings.HasPrefix(name, "mmcblk"):
		return "mmc"
	default:
		return "unknown"
	}
}

var partitionNamePattern = regexp.MustCompile(`^(?:(sd[a-z]+|vd[a-z]+|xvd[a-z]+|hd[a-z]+)([0-9]+)|(nvme[0-9]+n[0-9]+)p[0-9]+|(mmcblk[0-9]+)p[0-9]+)$`)

func partitionParent(name string) string {
	matches := partitionNamePattern.FindStringSubmatch(name)
	if len(matches) == 0 {
		return ""
	}
	for _, parent := range matches[1:] {
		if parent != "" {
			return parent
		}
	}
	return ""
}

func readInt(path string) (int64, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}
	value, err := strconv.ParseInt(strings.TrimSpace(string(data)), 10, 64)
	if err != nil || value < 0 {
		return 0, fmt.Errorf("invalid non-negative integer in %s", path)
	}
	return value, nil
}

type fsStats struct {
	TotalBytes     int64
	UsedBytes      int64
	FreeBytes      int64
	AvailableBytes int64
}

func statFS(path string) (fsStats, error) {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return fsStats{}, err
	}
	bs := int64(stat.Bsize)
	total := int64(stat.Blocks) * bs
	free := int64(stat.Bfree) * bs
	avail := int64(stat.Bavail) * bs
	used := total - free
	return fsStats{TotalBytes: total, UsedBytes: used, FreeBytes: free, AvailableBytes: avail}, nil
}

func detectFS(path string) string {
	var stat syscall.Statfs_t
	if err := syscall.Statfs(path, &stat); err != nil {
		return "unknown"
	}
	if stat.Type == unix.BTRFS_SUPER_MAGIC {
		return "btrfs"
	}
	if stat.Type == unix.EXT4_SUPER_MAGIC {
		return "ext4"
	}
	if stat.Type == unix.XFS_SUPER_MAGIC {
		return "xfs"
	}
	return "unknown"
}

func mountInfoForPath(path string) string {
	data, err := os.ReadFile("/proc/self/mountinfo")
	if err != nil {
		return "unknown"
	}
	for _, line := range strings.Split(string(data), "\n") {
		fields := strings.Fields(line)
		if len(fields) < 5 {
			continue
		}
		mountPoint := fields[4]
		if mountPoint == path || filepath.Clean(mountPoint) == filepath.Clean(path) {
			return mountPoint
		}
	}
	return "unknown"
}
