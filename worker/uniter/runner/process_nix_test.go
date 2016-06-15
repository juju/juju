// Copyright 2014 Canonical Ltd.
// Copyright 2014 Cloudbase Solutions SRL
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !windows

package runner_test

import (
	"syscall"
)

func processExists(pid int) bool {
	err := syscall.Kill(pid, 0)
	if err != nil {
		return false
	}
	return true
}
