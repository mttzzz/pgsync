//go:build !(windows && amd64) && !(linux && amd64) && !(darwin && amd64) && !(darwin && arm64)

package pgtools

import "runtime"

func currentEmbeddedBundle() embeddedBundle {
	return embeddedBundle{
		Platform:  platformName(runtime.GOOS, runtime.GOARCH),
		Version:   embeddedToolVersion,
		Available: false,
	}
}
