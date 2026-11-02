package hardware

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDetectMemoryReadsTotalAndAvailableRAM(t *testing.T) {
	proc := t.TempDir()
	meminfo := "MemTotal:       16384 kB\nMemAvailable:    8192 kB\nMemFree:         4096 kB\n"
	if err := os.WriteFile(filepath.Join(proc, "meminfo"), []byte(meminfo), 0o600); err != nil {
		t.Fatal(err)
	}
	got, _, err := detectMemory(DetectOptions{ProcRoot: proc})
	if err != nil {
		t.Fatal(err)
	}
	if got.TotalBytes != 16384*1024 || got.AvailableBytes != 8192*1024 || got.FreeBytes != 4096*1024 {
		t.Fatalf("unexpected memory values: %+v", got)
	}
}
