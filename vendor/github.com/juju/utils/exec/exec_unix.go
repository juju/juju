// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions SRL
// Licensed under the LGPLv3, see LICENCE file for details.

// +build !windows

package exec

import (
	"os"
	"syscall"
)

// KillProcess tries to kill the process being ran by RunParams
// We need this convoluted implementation because everything
// ran under the bash script is spawned as a different process
// and doesn't get killed by a regular process.Kill()
// For details see https://groups.google.com/forum/#!topic/golang-nuts/XoQ3RhFBJl8
func KillProcess(proc *os.Process) error {
	pgid, err := syscall.Getpgid(proc.Pid)
	if err == nil {
		return syscall.Kill(-pgid, 15) // note the minus sign
	}
	return nil
}

// populateSysProcAttr exists so that the method Kill on the same struct
// can work correctly. For more information see Kill's comment.
func (r *RunParams) populateSysProcAttr() {
	r.ps.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
}
