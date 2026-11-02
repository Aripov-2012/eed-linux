package version

import (
	"runtime"
	"testing"
)

func TestDetailsUsesBuildTargetOrRuntimeTarget(t *testing.T) {
	oldTarget := BuildTarget
	defer func() { BuildTarget = oldTarget }()
	BuildTarget = ""

	info := Details()
	if info.Version != Version {
		t.Fatalf("version mismatch: got %q, want %q", info.Version, Version)
	}
	if info.BuildTarget != runtime.GOOS+"/"+runtime.GOARCH {
		t.Fatalf("unexpected runtime build target %q", info.BuildTarget)
	}
	if info.ReleaseChannel == "" {
		t.Fatal("release channel must be present")
	}
}
