// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import (
	"time"

	"github.com/juju/juju/state/backups/metadata"
	"github.com/juju/juju/version"
)

// BackupsCreateArgs holds the args for the API Create method.
type BackupsCreateArgs struct {
	Notes string
}

// BackupsInfoArgs holds the args for the API Info method.
type BackupsInfoArgs struct {
	ID string
}

// BackupsListArgs holds the args for the API List method.
type BackupsListArgs struct {
}

// BackupsRemoveArgs holds the args for the API Remove method.
type BackupsRemoveArgs struct {
	ID string
}

// BackupsListResult holds the list of all stored backups.
type BackupsListResult struct {
	List []BackupsMetadataResult
}

// BackupsMetadataResult holds the metadata for a backup as returned by
// an API backups method (such as Create).
type BackupsMetadataResult struct {
	ID string

	Checksum       string
	ChecksumFormat string
	Size           int64
	Stored         time.Time // May be zero...

	Started     time.Time
	Finished    time.Time // May be zero...
	Notes       string
	Environment string
	Machine     string
	Hostname    string
	Version     version.Number
}

// UpdateFromMetadata updates the result with the information in the
// metadata value.
func (r *BackupsMetadataResult) UpdateFromMetadata(meta *metadata.Metadata) {
	r.ID = meta.ID()

	r.Checksum = meta.Checksum()
	r.ChecksumFormat = meta.ChecksumFormat()
	r.Size = meta.Size()
	if meta.Stored() != nil {
		r.Stored = *(meta.Stored())
	}

	r.Started = meta.Started
	if meta.Finished != nil {
		r.Finished = *meta.Finished
	}
	r.Notes = meta.Notes

	r.Environment = meta.Origin.Environment
	r.Machine = meta.Origin.Machine
	r.Hostname = meta.Origin.Hostname
	r.Version = meta.Origin.Version
}
