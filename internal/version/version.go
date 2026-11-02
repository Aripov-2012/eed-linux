package version

import (
	"fmt"
	"runtime"
)

var (
	Version        = "dev"
	Commit         = "unknown"
	BuildDate      = "unknown"
	BuildTarget    = ""
	ReleaseChannel = "development"
)

type Info struct {
	Version        string `json:"version"`
	Commit         string `json:"commit"`
	BuildDate      string `json:"build_date"`
	BuildTarget    string `json:"build_target"`
	ReleaseChannel string `json:"release_channel"`
}

func String() string {
	return Version
}

func Details() Info {
	target := BuildTarget
	if target == "" {
		target = runtime.GOOS + "/" + runtime.GOARCH
	}
	return Info{
		Version:        Version,
		Commit:         Commit,
		BuildDate:      BuildDate,
		BuildTarget:    target,
		ReleaseChannel: ReleaseChannel,
	}
}

func (i Info) String() string {
	return fmt.Sprintf("EED %s\nCommit: %s\nBuild date: %s\nTarget: %s\nChannel: %s",
		i.Version, i.Commit, i.BuildDate, i.BuildTarget, i.ReleaseChannel)
}
