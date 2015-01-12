// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// +build !linux

package backups

import (
	"os"
	"time"
)

// Backups only runs on state machines which only run Ubuntu.
// creationTime is stubbed out here so that the test suite will pass
// on non-linux (e.g. windows, darwin).

func creationTime(fi os.FileInfo) time.Time {
	return time.Time{}
}
