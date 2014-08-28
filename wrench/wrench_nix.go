// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.
// +build !windows

package wrench

import (
	"os"
	"syscall"
)

func isOwnedByJujuUser(fi os.FileInfo) bool {
	statStruct, ok := fi.Sys().(*syscall.Stat_t)
	if !ok {
		// Uid check is not supported on this platform so assume
		// the owner is ok.
		return true
	}
	return int(statStruct.Uid) == jujuUid
}
