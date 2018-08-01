// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions SRL
// Licensed under the LGPLv3, see LICENCE file for details.

// +build windows

package exec

import (
	"os"
)

// KillProcess tries to kill the process passed in.
func KillProcess(proc *os.Process) error {
	return proc.Kill()
}

// populateSysProcAttr is a noop on windows
func (r *RunParams) populateSysProcAttr() {}
