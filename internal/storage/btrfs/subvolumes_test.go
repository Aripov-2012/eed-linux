package btrfs

import (
	"context"
	"strings"
	"testing"
)

type subvolumeRunner struct {
	output string
	args   []string
}

func (r *subvolumeRunner) Run(_ context.Context, name string, args ...string) ([]byte, error) {
	r.args = append([]string{name}, args...)
	return []byte(r.output), nil
}

func TestListSubvolumesUsesReadOnlyMockedCommand(t *testing.T) {
	runner := &subvolumeRunner{output: "ID 256 gen 10 top level 5 path @home\n"}
	subvolumes, err := ListSubvolumesWithRunner(context.Background(), "/usr/bin/btrfs", "/", runner)
	if err != nil {
		t.Fatal(err)
	}
	if len(subvolumes) != 1 || subvolumes[0].Path != "@home" {
		t.Fatalf("unexpected subvolume layout: %+v", subvolumes)
	}
	if strings.Join(runner.args, " ") != "/usr/bin/btrfs subvolume list -o /" {
		t.Fatalf("unexpected read-only layout query: %#v", runner.args)
	}
}
