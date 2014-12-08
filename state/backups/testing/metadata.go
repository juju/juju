// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"time"

	"github.com/juju/juju/state/backups"
)

const (
	envID = "49db53ac-a42f-4ab2-86e1-0c6fa0fec762"
)

// NewMetadata returns a Metadata to use for testing.
func NewMetadata() *backups.Metadata {
	meta := NewMetadataStarted()

	timestamp := "20140924-010319"
	id := envID + "." + timestamp
	meta.SetID(id)

	FinishMetadata(meta)
	meta.SetStored(nil)
	return meta
}

// NewMetadataStarted returns a Metadata to use for testing.
func NewMetadataStarted() *backups.Metadata {
	meta := backups.NewMetadata()
	meta.Started = meta.Started.Truncate(time.Second)
	meta.Origin.Environment = envID
	meta.Origin.Machine = "0"
	meta.Origin.Hostname = "main-host"
	return meta
}

// FinishMetadata finishes a metadata with test values.
func FinishMetadata(meta *backups.Metadata) {
	var size int64 = 10
	checksum := "787b8915389d921fa23fb40e16ae81ea979758bf"
	meta.MarkComplete(size, checksum)
	finished := meta.Started.Add(time.Minute)
	meta.Finished = &finished
}

// UpdateNotes derives a new Metadata with new notes.
func UpdateNotes(meta *backups.Metadata, notes string) *backups.Metadata {
	copied := *meta
	copied.Notes = notes
	return &copied
}

// SetOrigin updates the metadata's origin with the privided information.
func SetOrigin(meta *backups.Metadata, envUUID, machine, hostname string) {
	meta.Origin.Environment = envUUID
	meta.Origin.Machine = machine
	meta.Origin.Hostname = hostname
}
