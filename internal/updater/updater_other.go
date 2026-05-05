//go:build !windows

package updater

import "os/exec"

func applyHiddenAttrs(_ *exec.Cmd) {}
