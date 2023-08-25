// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package looputil

import (
	"os"
	"syscall"
)

func fileInode(info os.FileInfo) uint64 {
	return info.Sys().(*syscall.Stat_t).Ino
}
