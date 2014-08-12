// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"time"

	"github.com/juju/juju/version"
)

const checksumFormat = "SHA-1, base64 encoded"

// BackupOrigin identifies where a backup archive came from.
type Origin struct {
	// Environment is the ID for the backed-up environment.
	Environment string
	// Machine is the ID of the state "machine" that ran the backup.
	Machine string
	// Hostname is where the backup happened.
	Hostname string
	// Version is the version of juju used to produce the backup.
	Version version.Number
}

// Metadata contains the metadata for a single state backup archive.
type Metadata struct {
	// ID is the unique ID assigned by the system.
	ID string
	// Timestamp records when the backup process was started for the archive.
	Timestamp time.Time
	// Finished records when the backup process finished for the archive.
	Finished time.Time
	// CheckSum is the checksum for the archive.
	CheckSum string
	// CheckSumFormat is the kind (and encoding) of checksum.
	CheckSumFormat string
	// Size is the size of the archive (in bytes).
	Size int64
	// Origin identifies where the backup was created.
	Origin Origin
	// Archived indicates whether or not the backup archive was stored.
	Stored bool
	// Notes (optional) contains any user-supplied annotations for the archive.
	Notes string // not required
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
		Timestamp: time.Now().UTC(),
		// Finished is omitted.
		CheckSum:       checksum,
		CheckSumFormat: checksumFormat,
		Size:           size,
		Origin:         origin,
		// Stored is left as false.
		Notes: notes,
	}
	return &metadata
}
