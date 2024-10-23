//go:build windows

// sys_windows.go
package utils

import (
	"os/exec"
	"syscall"
)

// SetSysProcAttr sets the SysProcAttr to hide the window
func SetSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
}
