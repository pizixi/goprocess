//go:build !windows

// sys_other.go
package utils

import (
	"os/exec"
	osuser "os/user"
	"strconv"
	"strings"
	"syscall"
)

// SetSysProcAttr sets the SysProcAttr field of the exec.Cmd struct
func SetSysProcAttr(cmd *exec.Cmd) {
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
}

// ConfigureProcessUser configures cmd to run as the requested Unix user.
func ConfigureProcessUser(cmd *exec.Cmd, username string) (func(), error) {
	username = strings.TrimSpace(username)
	if username == "" {
		return nil, nil
	}

	u, err := osuser.Lookup(username)
	if err != nil {
		return nil, err
	}

	uid, err := strconv.ParseUint(u.Uid, 10, 32)
	if err != nil {
		return nil, err
	}
	gid, err := strconv.ParseUint(u.Gid, 10, 32)
	if err != nil {
		return nil, err
	}

	var groups []uint32
	groupIDs, err := u.GroupIds()
	if err == nil {
		for _, groupID := range groupIDs {
			gidValue, err := strconv.ParseUint(groupID, 10, 32)
			if err != nil {
				continue
			}
			groups = append(groups, uint32(gidValue))
		}
	}

	SetSysProcAttr(cmd)
	cmd.SysProcAttr.Credential = &syscall.Credential{
		Uid:    uint32(uid),
		Gid:    uint32(gid),
		Groups: groups,
	}
	return nil, nil
}
