// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"os"
	"syscall"
	"time"
)

func creationTime(fi os.FileInfo) time.Time {
	rawstat := fi.Sys()
	if rawstat != nil {
		if stat, ok := rawstat.(*syscall.Stat_t); ok {
			return time.Unix(int64(stat.Ctim.Sec), 0)
		}
	}
	return time.Time{}
}
