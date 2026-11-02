package system

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDetectOSReadsUbuntuReleaseFieldsWithoutExecutingValues(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "os-release")
	marker := filepath.Join(dir, "should-not-exist")
	content := strings.Join([]string{
		`NAME="Ubuntu"`,
		`ID=ubuntu`,
		`ID_LIKE="debian"`,
		`VERSION_ID="24.04"`,
		`VERSION_CODENAME=noble`,
		`PRETTY_NAME="Ubuntu 24.04 LTS"`,
		`EVIL="$(touch ` + marker + `)"`,
	}, "\n")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}

	got, err := detectOS(path)
	if err != nil {
		t.Fatal(err)
	}
	if got.Name != "Ubuntu" || got.ID != "ubuntu" || got.IDLike != "debian" ||
		got.Version != "24.04" || got.VersionCodename != "noble" || got.PrettyName != "Ubuntu 24.04 LTS" {
		t.Fatalf("Ubuntu os-release fields were not preserved: %+v", got)
	}
	if _, err := os.Stat(marker); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("os-release values must not be executed: marker stat error=%v", err)
	}
}

func TestDetectStorageSeparatesOptionalMountErrorsFromRootFailure(t *testing.T) {
	mountInfo := "36 25 8:1 / / rw,relatime - ext4 /dev/sda1 rw\n" +
		"37 25 8:2 / /mnt/data rw,relatime - ext4 /dev/sdb1 rw\n"
	path := filepath.Join(t.TempDir(), "mountinfo")
	if err := os.WriteFile(path, []byte(mountInfo), 0o600); err != nil {
		t.Fatal(err)
	}
	opts := DetectOptions{
		MountInfoPath: path,
		StatFS: func(path string) (fsStats, error) {
			if path == "/mnt/data" {
				return fsStats{}, errors.New("permission denied")
			}
			return fsStats{TotalBytes: 1000, FreeBytes: 400, AvailableBytes: 300}, nil
		},
	}
	items, warnings, err := detectStorage(opts)
	if err != nil {
		t.Fatalf("optional mountpoint errors must not fail root detection: %v", err)
	}
	if len(items) != 2 || items[0].MountPoint != "/" || !items[0].CapacityKnown ||
		items[1].MountPoint != "/mnt/data" || items[1].CapacityKnown {
		t.Fatalf("real mounts should remain visible when optional capacity is unavailable: %+v", items)
	}
	if len(warnings) != 1 || !strings.Contains(warnings[0], "/mnt/data") {
		t.Fatalf("optional mountpoint failure should be reported: %+v", warnings)
	}

	opts.StatFS = func(path string) (fsStats, error) {
		if path == "/" {
			return fsStats{}, errors.New("inaccessible")
		}
		return fsStats{TotalBytes: 1000}, nil
	}
	items, _, err = detectStorage(opts)
	if err == nil || !strings.Contains(err.Error(), "required root filesystem") {
		t.Fatalf("root filesystem failure must remain explicit: items=%+v err=%v", items, err)
	}
}

func TestDetectBlockDevicesIncludesPartitionsAndUnknownSizes(t *testing.T) {
	root := filepath.Join(t.TempDir(), "sys", "class", "block")
	for _, name := range []string{"sda", "sda1", "mystery"} {
		if err := os.MkdirAll(filepath.Join(root, name), 0o700); err != nil {
			t.Fatal(err)
		}
		if name != "mystery" {
			if err := os.WriteFile(filepath.Join(root, name, "size"), []byte("2048\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
		if err := os.WriteFile(filepath.Join(root, name, "dev"), []byte("8:1\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(root, "sda1", "partition"), []byte("1\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	devices, err := detectBlockDevices(DetectOptions{SysfsRoot: filepath.Join(t.TempDir(), "missing")})
	if err == nil {
		t.Fatal("missing sysfs root should be reported")
	}
	if len(devices) != 0 {
		t.Fatalf("missing sysfs root should not invent devices: %+v", devices)
	}
	devices, err = detectBlockDevices(DetectOptions{SysfsRoot: filepath.Dir(filepath.Dir(root))})
	if err != nil {
		t.Fatal(err)
	}
	byName := make(map[string]BlockDevice)
	for _, device := range devices {
		byName[device.Name] = device
	}
	if len(byName) != 3 || !byName["sda"].SizeKnown || !byName["sda1"].IsPartition ||
		byName["sda1"].SizeBytes != 2048*512 || byName["mystery"].SizeKnown ||
		byName["sda"].Kind != "physical_disk" || byName["sda1"].Kind != "partition" ||
		byName["sda1"].Parent != "sda" {
		t.Fatalf("block device inventory did not preserve partition/size state: %+v", devices)
	}
}

func TestDetectBlockDeviceKindsAndTransports(t *testing.T) {
	dir := t.TempDir()
	sysfs := filepath.Join(dir, "sys")
	blockRoot := filepath.Join(sysfs, "class", "block")
	devices := map[string]struct {
		kind      string
		transport string
		path      string
		partition bool
		parent    string
	}{
		"nvme0n1":   {kind: "physical_disk", transport: "nvme", path: "devices/pci/nvme/nvme0/nvme0n1"},
		"nvme0n1p1": {kind: "partition", transport: "nvme", path: "devices/pci/nvme/nvme0/nvme0n1/nvme0n1p1", partition: true, parent: "nvme0n1"},
		"sda":       {kind: "physical_disk", transport: "sata", path: "devices/pci/ata1/host0/block/sda"},
		"sdb":       {kind: "physical_disk", transport: "usb", path: "devices/pci/usb1/1-1/host1/block/sdb"},
		"vda":       {kind: "virtual", transport: "virtio", path: "devices/pci/virtio0/block/vda"},
		"xvda":      {kind: "virtual", transport: "xen", path: "devices/vbd-51712/block/xvda"},
		"loop0":     {kind: "loop", transport: "unknown", path: "devices/virtual/block/loop0"},
		"dm-0":      {kind: "device_mapper", transport: "unknown", path: "devices/virtual/block/dm-0"},
		"zram0":     {kind: "virtual", transport: "unknown", path: "devices/virtual/block/zram0"},
	}
	for name, expected := range devices {
		target := filepath.Join(sysfs, expected.path)
		if err := os.MkdirAll(target, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.MkdirAll(blockRoot, 0o700); err != nil {
			t.Fatal(err)
		}
		if err := os.Symlink(target, filepath.Join(blockRoot, name)); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(filepath.Join(target, "size"), []byte("4096\n"), 0o600); err != nil {
			t.Fatal(err)
		}
		if expected.partition {
			if err := os.WriteFile(filepath.Join(target, "partition"), []byte("1\n"), 0o600); err != nil {
				t.Fatal(err)
			}
		}
	}
	got, err := detectBlockDevices(DetectOptions{SysfsRoot: sysfs})
	if err != nil {
		t.Fatal(err)
	}
	for _, device := range got {
		expected := devices[device.Name]
		if device.Kind != expected.kind || device.Transport != expected.transport ||
			device.IsPartition != expected.partition || device.Parent != expected.parent ||
			device.SizeBytes != 4096*512 {
			t.Errorf("%s classification mismatch: got %+v want %+v", device.Name, device, expected)
		}
	}
}

func TestDetectStorageDistinguishesRealAndSpecialMounts(t *testing.T) {
	tests := []struct {
		name       string
		mountInfo  string
		homeDir    string
		wantPaths  []string
		wantKinds  map[string]string
		wantBind   bool
		wantStatFS int
	}{
		{
			name:      "root filesystem",
			mountInfo: "36 25 8:1 / / rw,relatime - ext4 /dev/sda1 rw\n",
			wantPaths: []string{"/"}, wantKinds: map[string]string{"/": "root"}, wantStatFS: 1,
		},
		{
			name: "separate home filesystem",
			mountInfo: "36 25 8:1 / / rw,relatime - ext4 /dev/sda1 rw\n" +
				"37 25 8:2 / /home rw,relatime - ext4 /dev/sdb1 rw\n",
			homeDir: "/home/user", wantPaths: []string{"/", "/home"},
			wantKinds: map[string]string{"/": "root", "/home": "home"}, wantStatFS: 2,
		},
		{
			name: "bind mounted home",
			mountInfo: "36 25 8:1 / / rw,relatime - ext4 /dev/sda1 rw\n" +
				"37 25 8:1 /home /home rw,relatime - ext4 /dev/sda1 rw\n",
			homeDir: "/home/user", wantPaths: []string{"/", "/home"},
			wantKinds: map[string]string{"/": "root", "/home": "bind"}, wantBind: true, wantStatFS: 2,
		},
		{
			name: "btrfs home subvolume",
			mountInfo: "36 25 8:1 /@ / rw,relatime - btrfs /dev/sda1 rw\n" +
				"37 25 8:1 /@home /home rw,relatime - btrfs /dev/sda1 rw\n",
			homeDir: "/home/user", wantPaths: []string{"/", "/home"},
			wantKinds: map[string]string{"/": "root", "/home": "home"}, wantStatFS: 2,
		},
		{
			name:      "ordinary home directory is not a mount",
			mountInfo: "36 25 8:1 / / rw,relatime - ext4 /dev/sda1 rw\n",
			homeDir:   "/home/user", wantPaths: []string{"/"},
			wantKinds: map[string]string{"/": "root"}, wantStatFS: 1,
		},
		{
			name: "nested data mounts",
			mountInfo: "36 25 8:1 / / rw,relatime - ext4 /dev/sda1 rw\n" +
				"37 25 8:2 / /mnt/data rw,relatime - ext4 /dev/sdb1 rw\n" +
				"38 37 8:3 / /mnt/data/archive rw,relatime - xfs /dev/sdc1 rw\n",
			wantPaths:  []string{"/", "/mnt/data", "/mnt/data/archive"},
			wantKinds:  map[string]string{"/": "root", "/mnt/data": "local", "/mnt/data/archive": "local"},
			wantStatFS: 3,
		},
		{
			name: "remote data filesystem retains capacity",
			mountInfo: "36 25 8:1 / / rw,relatime - ext4 /dev/sda1 rw\n" +
				"37 25 0:44 / /mnt/shared rw,relatime - nfs4 server:/export rw\n",
			wantPaths:  []string{"/", "/mnt/shared"},
			wantKinds:  map[string]string{"/": "root", "/mnt/shared": "remote"},
			wantStatFS: 2,
		},
		{
			name: "EFI snap overlay fuse and pseudo mounts",
			mountInfo: "36 25 8:1 / / rw,relatime - ext4 /dev/sda1 rw\n" +
				"37 25 8:2 / /boot/efi rw,relatime - vfat /dev/sda2 rw\n" +
				"38 25 7:0 / /snap/core/123 ro,nodev - squashfs /dev/loop0 ro\n" +
				"39 25 0:42 / /var/lib/docker/overlay2 rw - overlay overlay rw\n" +
				"40 25 0:52 / /run/user/1000/gvfs rw - fuse.gvfsd-fuse gvfs rw\n" +
				"41 25 0:3 / /proc rw - proc proc rw\n" +
				"42 25 0:54 / /run/docker/netns/test rw - nsfs nsfs rw\n" +
				"43 25 0:55 / /mnt/image rw - squashfs /dev/sr0 ro\n",
			wantPaths: []string{"/", "/boot/efi", "/snap/core/123", "/var/lib/docker/overlay2", "/run/user/1000/gvfs", "/proc", "/run/docker/netns/test", "/mnt/image"},
			wantKinds: map[string]string{
				"/": "root", "/boot/efi": "efi", "/snap/core/123": "snap",
				"/var/lib/docker/overlay2": "overlay", "/run/user/1000/gvfs": "fuse", "/proc": "pseudo",
				"/run/docker/netns/test": "namespace", "/mnt/image": "virtual",
			},
			wantStatFS: 3,
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "mountinfo")
			if err := os.WriteFile(path, []byte(test.mountInfo), 0o600); err != nil {
				t.Fatal(err)
			}
			statCalls := 0
			items, _, err := detectStorage(DetectOptions{
				MountInfoPath: path, HomeDir: test.homeDir,
				StatFS: func(string) (fsStats, error) {
					statCalls++
					return fsStats{TotalBytes: 1000, AvailableBytes: 500}, nil
				},
			})
			if err != nil {
				t.Fatal(err)
			}
			if len(items) != len(test.wantPaths) || statCalls != test.wantStatFS {
				t.Fatalf("mount set/capacity probes mismatch: mounts=%+v stat calls=%d", items, statCalls)
			}
			for index, item := range items {
				if item.MountPoint != test.wantPaths[index] || item.Category != test.wantKinds[item.MountPoint] {
					t.Errorf("mount %d = %+v, want path=%s category=%s", index, item,
						test.wantPaths[index], test.wantKinds[test.wantPaths[index]])
				}
			}
			if test.wantBind && (!items[1].BindMount || items[1].Root != "/home") {
				t.Errorf("bind-mounted /home was not represented accurately: %+v", items[1])
			}
			for _, item := range items {
				if item.Category == "snap" || item.Category == "overlay" || item.Category == "pseudo" ||
					item.Category == "namespace" || item.Category == "virtual" {
					if item.CapacityKnown {
						t.Errorf("%s should not be reported as independent storage capacity: %+v", item.Category, item)
					}
				}
			}
		})
	}
}
