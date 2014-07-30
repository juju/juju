// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !windows

package container

import (
	"syscall"
)

func flock(fd int, how int) (err error) {
	return syscall.Flock(fd, how)
}
