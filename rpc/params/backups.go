// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package params

import (
	"time"

	corebackups "github.com/juju/juju/core/backups"
	"github.com/juju/juju/core/semversion"
)

// BackupsCreateArgs holds the args for the API Create method.
type BackupsCreateArgs struct {
	Notes string `json:"notes"`

	// NoDownload is kept for compatibility with older clients only.
	// The controller no longer supports keeping the archive instead of
	// downloading it, so requests that set this field are rejected. The
	// field is still always serialized so the wire format of the
	// shipped Backups facade versions is unchanged.
	NoDownload bool `json:"no-download"`
}

// BackupsDownloadArgs holds the args for the HTTP backups download endpoint.
// The ID is an opaque server-minted identifier; it is never treated as a
// filesystem path.
type BackupsDownloadArgs struct {
	ID string `json:"id"`
}

// BackupsMetadataResult holds the metadata for a backup as returned by
// an API backups method (such as Create).
type BackupsMetadataResult struct {
	// ID identifies the backup. For the Backups facade's Create method
	// the field is overloaded: the controller returns the id of the
	// one-shot download staged for the backup, a server-minted UUID
	// that is valid only until the backup-download-ttl retention
	// window lapses, not a persistent backup identity.
	ID string `json:"id"`

	Checksum       string    `json:"checksum"`
	ChecksumFormat string    `json:"checksum-format"`
	Size           int64     `json:"size"`
	Stored         time.Time `json:"stored"` // May be zero...

	Started  time.Time         `json:"started"`
	Finished time.Time         `json:"finished"` // May be zero...
	Notes    string            `json:"notes"`
	Model    string            `json:"model"`
	Machine  string            `json:"machine"`
	Hostname string            `json:"hostname"`
	Version  semversion.Number `json:"version"`
	Base     string            `json:"base"`

	Filename string `json:"filename"`

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

// CreateResult updates the result with the information in the
// metadata value.
func CreateResult(meta *corebackups.Metadata, filename string) BackupsMetadataResult {
	var result BackupsMetadataResult

	result.ID = meta.ID()

	result.Checksum = meta.Checksum()
	result.ChecksumFormat = meta.ChecksumFormat()
	result.Size = meta.Size()
	if meta.Stored() != nil {
		result.Stored = *(meta.Stored())
	}

	result.Started = meta.Started
	if meta.Finished != nil {
		result.Finished = *meta.Finished
	}
	result.Notes = meta.Notes

	result.Model = meta.Origin.Model
	result.Machine = meta.Origin.Machine
	result.Hostname = meta.Origin.Hostname
	result.Version = meta.Origin.Version
	result.Base = meta.Origin.Base

	result.ControllerUUID = meta.Controller.UUID
	result.FormatVersion = meta.FormatVersion
	result.HANodes = meta.Controller.HANodes
	result.ControllerMachineID = meta.Controller.MachineID
	result.ControllerMachineInstanceID = meta.Controller.MachineInstanceID
	result.Filename = filename

	return result
}
