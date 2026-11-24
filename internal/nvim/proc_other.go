//go:build !windows

package nvim

import "os/exec"

func hideConsole(cmd *exec.Cmd) {}
