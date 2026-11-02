package eed

import (
	"embed"
	"io/fs"
)

//go:embed components/manifests/*.toml
var builtinManifestFiles embed.FS

func BuiltinManifests() fs.FS {
	return builtinManifestFiles
}
