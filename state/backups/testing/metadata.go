// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"time"

	"github.com/juju/juju/state/backups/metadata"
)

const (
	// ID is the backups ID used by the metadata helper functions.
	ID = "49db53ac-a42f-4ab2-86e1-0c6fa0fec762.20140924-010319"
	// EnvID is the env ID used by metadata helper functions.
	EnvID = "49db53ac-a42f-4ab2-86e1-0c6fa0fec762"
	// Machine is the machine ID used by metadata helper functions.
	Machine = "0"
	// Hostname is the hostname used by metadata helper functions.
	Hostname = "main-host"
	// Notes is the notes value used by metadata helper functions.
	Notes = ""
	// Size is the size used by metadata helper functions.
	Size = 10
	// Checksum is the checksum used by metadata helper functions.
	Checksum = "787b8915389d921fa23fb40e16ae81ea979758bf"
	// CsFormat is the checksum format used by metadata helper functions.
	CsFormat = metadata.ChecksumFormat
)

// NewMetadata returns a Metadata to use for testing.
func NewMetadata() *metadata.Metadata {
	meta := NewMetadataStarted(ID, Notes)
	FinishMetadata(meta)
	meta.SetStored()
	return meta
}

// NewMetadataStarted returns a Metadata to use for testing.
func NewMetadataStarted(id, notes string) *metadata.Metadata {
	origin := metadata.NewOrigin(EnvID, Machine, Hostname)
	started := time.Now().UTC()

	meta := metadata.NewMetadata(*origin, notes, &started)
	meta.SetID(id)
	return meta
}

// FinishMetadata finishes a metadata with test values.
func FinishMetadata(meta *metadata.Metadata) {
	finished := meta.Started().Add(time.Minute)
	meta.Finish(Size, Checksum, CsFormat, &finished)
}

// UpdateNotes derives a new Metadata with new notes.
func UpdateNotes(meta *metadata.Metadata, notes string) *metadata.Metadata {
	started := meta.Started()
	newMeta := metadata.NewMetadata(meta.Origin(), notes, &started)
	newMeta.SetID(meta.ID())
	newMeta.Finish(meta.Size(), meta.Checksum(), meta.ChecksumFormat(), meta.Finished())
	if meta.Stored() {
		newMeta.SetStored()
	}
	return newMeta
}
