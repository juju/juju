// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import (
	"time"

	"github.com/juju/names"

	"github.com/juju/juju/instance"
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

// BackupsDownloadArgs holds the args for the API Download method.
type BackupsDownloadArgs struct {
	ID string
}

// BackupsUploadArgs holds the args for the API Upload method.
type BackupsUploadArgs struct {
	Data     []byte
	Metadata BackupsMetadataResult
}

// BackupsRemoveArgs holds the args for the API Remove method.
type BackupsRemoveArgs struct {
	ID string
}

// BackupsListResult holds the list of all stored backups.
type BackupsListResult struct {
	List []BackupsMetadataResult
}

// BackupsListResult holds the list of all stored backups.
type BackupsUploadResult struct {
	ID string
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

// RestoreArgs holds the args to be used to call state/backups.Restore
type RestoreArgs struct {
	PrivateAddress string
	NewInstId      instance.Id
	NewInstTag     names.Tag
	NewInstSeries  string
}

// RestoreAPIArgs Holds the backup file or id and the machine to
// be used for the restore process.
type RestoreAPIArgs struct {
	// BackupId holds the id of the backup in server if any
	BackupId string
	// Machine holds the machine where the backup is going to be restored
	Machine string
}
