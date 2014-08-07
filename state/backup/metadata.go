// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backup

import (
	"time"

	"github.com/juju/juju/version"
)

const checksumFormat = "SHA-1, base64 encoded"

// BackupOrigin identifies where a backup archive came from.
//
// Environment is the ID for the backed-up environment.
// Machine is the ID of the state "machine" that ran the backup.
// Hostname is where the backup happened.
// Version is the version of juju used to produce the backup.
type Origin struct {
	Environment string
	Machine     string
	Hostname    string
	Version     version.Number
}

// Metadata contains the metadata for a single state backup archive.
//
// ID is the unique ID assigned by the system.
// Notes (optional) contains any user-supplied annotations for the archive.
// Timestamp records when the backup process was started for the archive.
// CheckSum is the checksum for the archive.
// CheckSumFormat is the kind (and encoding) of checksum.
// Size is the size of the archive (in bytes).
// Origin identifies where the backup was created.
// Archived indicates whether or not the backup archive was stored.
type Metadata struct {
	ID             string
	Notes          string // not required
	Timestamp      time.Time
	Finished       time.Time
	CheckSum       string
	CheckSumFormat string
	Size           int64
	Origin         Origin
	Archived       bool
}

// NewMetadata returns a new Metadata for a state backup archive.  The
// current date/time is used for the timestamp and the default checksum
// format is used.  ID is not set.  That is left up to the persistence
// layer.  Archived is set as false.  "notes" may be empty, but
// everything else should be provided.
func NewMetadata(
	checksum string, size int64, origin Origin, notes string,
) *Metadata {
	metadata := Metadata{
		// ID is omitted.
		Notes:     notes,
		Timestamp: time.Now().UTC(),
		// Finished is omitted.
		CheckSum:       checksum,
		CheckSumFormat: checksumFormat,
		Size:           size,
		Origin:         origin,
		// Archived is left as false.
	}
	return &metadata
}
