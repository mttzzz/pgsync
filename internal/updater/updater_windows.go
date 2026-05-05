//go:build windows

package updater

import (
	"os/exec"
	"syscall"
)

// CREATE_NO_WINDOW: child runs without an attached console — no flashing
// cmd.exe window when the update helper kicks off.
const createNoWindow = 0x08000000

func applyHiddenAttrs(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{
		HideWindow:    true,
		CreationFlags: createNoWindow,
	}
}
