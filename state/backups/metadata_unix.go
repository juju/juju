// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !windows

package backups

import (
	"os"
	"syscall"
	"time"
)

func creationTime(fi os.FileInfo) time.Time {
	var timestamp time.Time
	rawstat := fi.Sys()
	if rawstat != nil {
		stat, ok := rawstat.(*syscall.Stat_t)
		if ok {
			timestamp = time.Unix(int64(stat.Ctim.Sec), 0)
		}
	}
	return timestamp
}
