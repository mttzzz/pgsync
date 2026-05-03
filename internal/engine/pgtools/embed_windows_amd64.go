//go:build windows && amd64

package pgtools

import "embed"

//go:embed bin/windows-amd64/*
var windowsAMD64PgtoolsFS embed.FS

func currentEmbeddedBundle() embeddedBundle {
	return newEmbeddedBundle("windows", "amd64", windowsAMD64PgtoolsFS, "bin/windows-amd64")
}
