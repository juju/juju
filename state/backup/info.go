// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backup

import (
	"time"

	"github.com/juju/utils"

	"github.com/juju/juju/version"
)

type Status string

const (
	StatusNotSet         Status = ""
	StatusAvailable      Status = "available"
	StatusInfoOnly       Status = "info-only"
	StatusBuilding       Status = "building"
	StatusStoringInfo    Status = "storing"
	StatusStoringArchive Status = "storing"
	StatusFailed         Status = "failed"
)

const IDFormat = "%Y%M%D-%h%m%s"

func IDFromTimestamp(timestamp *time.Time, ver *version.Number) string {
	format := IDFormat
	// If the default format changes in a future version we will need to
	// look up the old format based on the Major/Minor of the version.
	return utils.FormatTimestamp(format, timestamp)
}

func TimestampFromID(id string, ver *version.Number) *time.Time {
	format := IDFormat
	// If the default format changes in a future version we will need to
	// look up the old format based on the Major/Minor of the version.
	return utils.ParseTimestamp(format, id)
}

type Info struct {
	ID        string
	Notes     string
	Timestamp *time.Time
	CheckSum  string // SHA-1
	Size      int64
	Version   *version.Number
	Status    Status
}
