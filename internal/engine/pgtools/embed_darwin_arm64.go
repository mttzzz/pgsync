//go:build darwin && arm64

package pgtools

import "embed"

//go:embed bin/darwin-arm64/*
var darwinARM64PgtoolsFS embed.FS

func currentEmbeddedBundle() embeddedBundle {
	return newEmbeddedBundle("darwin", "arm64", darwinARM64PgtoolsFS, "bin/darwin-arm64")
}
