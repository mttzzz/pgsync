//go:build darwin && amd64

package pgtools

import "embed"

//go:embed bin/darwin-amd64/*
var darwinAMD64PgtoolsFS embed.FS

func currentEmbeddedBundle() embeddedBundle {
	return newEmbeddedBundle("darwin", "amd64", darwinAMD64PgtoolsFS, "bin/darwin-amd64")
}
