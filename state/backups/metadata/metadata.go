// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package metadata

import (
	"bytes"
	"encoding/json"
	"io"
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils/filestorage"

	"github.com/juju/juju/version"
)

// ChecksumFormat identifies how to interpret the checksum for a backup
// generated with this version of juju.
const ChecksumFormat = "SHA-1, base64 encoded"

// Metadata contains the metadata for a single state backup archive.
type Metadata struct {
	filestorage.FileMetadata

	// Started records when the backup was started.
	Started time.Time
	// Finished records when the backup was complete.
	Finished *time.Time
	// Origin identifies where the backup was created.
	Origin Origin
	// Notes is an optional user-supplied annotation.
	Notes string
}

// NewMetadata returns a new Metadata for a state backup archive.  The
// current date/time is used for the timestamp and the default checksum
// format is used.  ID is not set.  That is left up to the persistence
// layer.  Stored is set as false.  "notes" may be empty, but
// everything else should be provided.
func NewMetadata(origin Origin, notes string, started *time.Time) *Metadata {
	filemeta := filestorage.NewMetadata()
	meta := Metadata{
		FileMetadata: *filemeta,
		Origin:       origin,
		Notes:        notes,
	}

	if started == nil {
		meta.Started = time.Now().UTC()
	} else {
		meta.Started = *started
	}

	return &meta
}

// Finish populates the remaining metadata values.  If format is empty,
// it is set to the default checksum format.  If finished is nil, it is
// set to the current time.
func (m *Metadata) Finish(size int64, checksum string) error {
	if size == 0 {
		return errors.New("missing size")
	}
	if checksum == "" {
		return errors.New("missing checksum")
	}
	format := ChecksumFormat
	finished := time.Now().UTC()

	if err := m.SetFile(size, checksum, format); err != nil {
		return errors.Annotate(err, "unexpected failure")
	}
	m.Finished = &finished

	return nil
}

// Copy returns a deep copy of the metadata.
func (m *Metadata) Copy() filestorage.Document {
	fileMeta := m.FileMetadata.Copy().(*filestorage.FileMetadata)
	copied := *m
	copied.FileMetadata = *fileMeta
	return &copied
}

type flatMetadata struct {
	ID string

	// file storage

	Checksum       string
	ChecksumFormat string
	Size           int64
	Stored         time.Time

	// backup

	Started     time.Time
	Finished    time.Time
	Notes       string
	Environment string
	Machine     string
	Hostname    string
	Version     version.Number
}

// TODO(ericsnow) Move AsJSONBuffer to filestorage.Metadata.

// AsJSONBuffer returns a bytes.Buffer containing the JSON-ified metadata.
func (m *Metadata) AsJSONBuffer() (io.Reader, error) {
	flat := flatMetadata{
		ID: m.ID(),

		Checksum:       m.Checksum(),
		ChecksumFormat: m.ChecksumFormat(),
		Size:           m.Size(),

		Started:     m.Started,
		Notes:       m.Notes,
		Environment: m.Origin.Environment,
		Machine:     m.Origin.Machine,
		Hostname:    m.Origin.Hostname,
		Version:     m.Origin.Version,
	}

	stored := m.Stored()
	if stored != nil {
		flat.Stored = *stored
	}

	if m.Finished != nil {
		flat.Finished = *m.Finished
	}

	var outfile bytes.Buffer
	if err := json.NewEncoder(&outfile).Encode(flat); err != nil {
		return nil, errors.Trace(err)
	}
	return &outfile, nil
}
