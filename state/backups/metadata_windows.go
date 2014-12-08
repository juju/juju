// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !windows

package backups

import (
	"os"
	"time"
)

func creationTime(fi os.FileInfo) time.Time {
	return time.Time{}
}
