// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backup

import (
	"time"

	"github.com/juju/juju/version"
)

type BackupInfo struct {
	Name      string
	Timestamp time.Time
	CheckSum  string // SHA-1
	Size      int64
	Version   version.Number
}
