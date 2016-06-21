// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import (
	"time"

	"github.com/juju/version"
)

// BackupsCreateArgs holds the args for the API Create method.
type BackupsCreateArgs struct {
	Notes string `json:"notes"`
}

// BackupsInfoArgs holds the args for the API Info method.
type BackupsInfoArgs struct {
	ID string `json:"id"`
}

// BackupsListArgs holds the args for the API List method.
type BackupsListArgs struct {
}

// BackupsDownloadArgs holds the args for the API Download method.
type BackupsDownloadArgs struct {
	ID string `json:"id"`
}

// BackupsUploadArgs holds the args for the API Upload method.
type BackupsUploadArgs struct {
	Data     []byte                `json:"data"`
	Metadata BackupsMetadataResult `json:"metadata"`
}

// BackupsRemoveArgs holds the args for the API Remove method.
type BackupsRemoveArgs struct {
	ID string `json:"id"`
}

// BackupsListResult holds the list of all stored backups.
type BackupsListResult struct {
	List []BackupsMetadataResult `json:"list"`
}

// BackupsListResult holds the list of all stored backups.
type BackupsUploadResult struct {
	ID string `json:"id"`
}

// BackupsMetadataResult holds the metadata for a backup as returned by
// an API backups method (such as Create).
type BackupsMetadataResult struct {
	ID string `json:"id"`

	Checksum       string    `json:"checksum"`
	ChecksumFormat string    `json:"checksum-format"`
	Size           int64     `json:"size"`
	Stored         time.Time `json:"stored"` // May be zero...

	Started  time.Time      `json:"started"`
	Finished time.Time      `json:"finished"` // May be zero...
	Notes    string         `json:"notes"`
	Model    string         `json:"model"`
	Machine  string         `json:"machine"`
	Hostname string         `json:"hostname"`
	Version  version.Number `json:"version"`
	Series   string         `json:"series"`

	CACert       string `json:"ca-cert"`
	CAPrivateKey string `json:"ca-private-key"`
}

// RestoreArgs Holds the backup file or id
type RestoreArgs struct {
	// BackupId holds the id of the backup in server if any
	BackupId string `json:"backup-id"`
}
