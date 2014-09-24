// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"time"

	"github.com/juju/juju/state/backups/metadata"
)

const (
	EnvID    = "49db53ac-a42f-4ab2-86e1-0c6fa0fec762"
	Machine  = "0"
	Hostname = "main-host"
	Notes    = ""
	Size     = 10
	Checksum = "787b8915389d921fa23fb40e16ae81ea979758bf"
)

var (
	ID       = EnvID + ".20140924-010319"
	CsFormat = metadata.ChecksumFormat
	Started  = time.Now().UTC()
	Finished = Started.Add((time.Duration)(time.Minute))
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

	meta := metadata.NewMetadata(*origin, notes, &Started)
	meta.SetID(id)
	return meta
}

// FinishMetadata finishes a metadata with test values.
func FinishMetadata(meta *metadata.Metadata) {
	meta.Finish(Size, Checksum, CsFormat, &Finished)
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
