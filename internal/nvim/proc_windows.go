package nvim

import (
	"os/exec"
	"syscall"
)

const createNoWindow = 0x08000000

// hideConsole keeps nvim.exe (a console program) from opening a console
// window next to the GUI.
func hideConsole(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true, CreationFlags: createNoWindow}
}
