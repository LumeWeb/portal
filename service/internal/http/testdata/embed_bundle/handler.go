package embed_bundle

import (
	"embed"
	"io/fs"
)

//go:embed all:build/*
var appFs embed.FS

func GetFS() fs.FS {
	appFiles, _ := fs.Sub(appFs, "build")

	return appFiles
}
