//go:build linux && amd64

package pgtools

import "embed"

//go:embed bin/linux-amd64/*
var linuxAMD64PgtoolsFS embed.FS

func currentEmbeddedBundle() embeddedBundle {
	return newEmbeddedBundle("linux", "amd64", linuxAMD64PgtoolsFS, "bin/linux-amd64")
}
