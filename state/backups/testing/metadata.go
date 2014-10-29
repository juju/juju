// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"time"

	"github.com/juju/juju/state/backups/metadata"
)

const (
	envID = "49db53ac-a42f-4ab2-86e1-0c6fa0fec762"
)

// NewMetadata returns a Metadata to use for testing.
func NewMetadata() *metadata.Metadata {
	timestamp := "20140924-010319"
	id := envID + "." + timestamp
	notes := ""
	meta := NewMetadataStarted(id, notes)

	FinishMetadata(meta)
	meta.SetStored(nil)
	return meta
}

// NewMetadataStarted returns a Metadata to use for testing.
func NewMetadataStarted(id, notes string) *metadata.Metadata {
	machine := "0"
	hostname := "main-host"
	origin := metadata.NewOrigin(envID, machine, hostname)
	started := time.Now().UTC()

	meta := metadata.NewMetadata(*origin, notes, &started)
	meta.SetID(id)
	return meta
}

// FinishMetadata finishes a metadata with test values.
func FinishMetadata(meta *metadata.Metadata) {
	var size int64 = 10
	checksum := "787b8915389d921fa23fb40e16ae81ea979758bf"
	meta.Finish(size, checksum)
	finished := meta.Started.Add(time.Minute)
	meta.Finished = &finished
}

// UpdateNotes derives a new Metadata with new notes.
func UpdateNotes(meta *metadata.Metadata, notes string) *metadata.Metadata {
	copied := meta.Copy().(*metadata.Metadata)
	copied.SetID(meta.ID())
	copied.Notes = notes
	return copied
}
