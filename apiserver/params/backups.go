// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import (
	"time"

	"github.com/juju/version/v2"
)

// BackupsCreateArgs holds the args for the API Create method.
type BackupsCreateArgs struct {
	Notes      string `json:"notes"`
	KeepCopy   bool   `json:"keep-copy"`
	NoDownload bool   `json:"no-download"`
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
	IDs []string `json:"ids"`
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
	Filename     string `json:"filename"`

	// FormatVersion stores the version of the backup format.
	// All unversioned backup files are considered 0,
	// so the versioned formats start at 1.
	FormatVersion int64 `json:"format-version"`

	// ControllerUUID is the controller UUID that is backed up.
	ControllerUUID string `json:"controller-uuid"`

	// ControllerMachineID is the controller machine ID that the backup was created on.
	ControllerMachineID string `json:"controller-machine-id"`

	// ControllerMachineInstanceID is the controller machine cloud instance ID that the backup was created on.
	ControllerMachineInstanceID string `json:"controller-machine-inst-id"`

	// HANodes reflects HA configuration: number of controller nodes in HA.
	HANodes int64 `json:"ha-nodes"`
}

// RestoreArgs Holds the backup file or id
type RestoreArgs struct {
	// BackupId holds the id of the backup in server if any
	BackupId string `json:"backup-id"`
}
