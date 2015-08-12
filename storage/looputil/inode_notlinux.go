// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

//+build !linux

package looputil

import (
	"os"
	"runtime"
)

func fileInode(os.FileInfo) uint64 {
	panic("loop devices not supported on " + runtime.GOOS)
}
