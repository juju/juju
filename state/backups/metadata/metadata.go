// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metadata

import (
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils/filestorage"
)

const checksumFormat = "SHA-1, base64 encoded"

// Metadata contains the metadata for a single state backup archive.
type Metadata struct {
	filestorage.FileMetadata
	finished *time.Time
	origin   Origin
	notes    string // not required
}

// NewMetadata returns a new Metadata for a state backup archive.  The
// current date/time is used for the timestamp and the default checksum
// format is used.  ID is not set.  That is left up to the persistence
// layer.  Stored is set as false.  "notes" may be empty, but
// everything else should be provided.
func NewMetadata(origin Origin, notes string, started *time.Time) *Metadata {
	raw := filestorage.NewMetadata(started)
	metadata := Metadata{*raw, nil, origin, notes}
	return &metadata
}

// Started records when the backup process started for the archive.
func (m *Metadata) Started() time.Time {
	return m.Timestamp()
}

// Finished records when the backup process finished for the archive.
func (m *Metadata) Finished() *time.Time {
	return m.finished
}

// Origin identifies where the backup was created.
func (m *Metadata) Origin() Origin {
	return m.origin
}

// Notes contains user-supplied annotations for the archive, if any.
func (m *Metadata) Notes() string {
	return m.notes
}

// Finish populates the remaining metadata values.  If format is empty,
// it is set to the default checksum format.  If finished is nil, it is
// set to the current time.
func (m *Metadata) Finish(size int64, checksum, format string, finished *time.Time) error {
	if size == 0 {
		return errors.New("missing size")
	}
	if checksum == "" {
		return errors.New("missing checksum")
	}
	if format == "" {
		format = checksumFormat
	}
	if finished == nil {
		now := time.Now().UTC()
		finished = &now
	}

	if err := m.SetFile(size, checksum, checksumFormat); err != nil {
		return errors.Annotate(err, "unexpected failure")
	}
	m.finished = finished

	return nil
}
