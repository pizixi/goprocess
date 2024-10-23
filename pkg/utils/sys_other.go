//go:build !windows

// sys_other.go
package utils

import (
	"os/exec"
	"syscall"
)

// SetSysProcAttr sets the SysProcAttr field of the exec.Cmd struct
func SetSysProcAttr(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{}
}
