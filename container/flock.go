// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !windows

package container

import (
	"syscall"
)

func flockLock(fd int) (err error) {
	return syscall.Flock(fd, syscall.LOCK_EX)
}

func flockUnlock(fd int) (err error) {
	return syscall.Flock(fd, syscall.LOCK_UN)
}
