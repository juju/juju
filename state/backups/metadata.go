// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"bytes"
	"encoding/json"
	"io"
	"time"

	"github.com/juju/errors"
	"github.com/juju/utils/filestorage"

	"github.com/juju/juju/version"
)

// checksumFormat identifies how to interpret the checksum for a backup
// generated with this version of juju.
const checksumFormat = "SHA-1, base64 encoded"

// Origin identifies where a backup archive came from.  While it is
// more about where and Metadata about what and when, that distinction
// does not merit special consideration.  Instead, Origin exists
// separately from Metadata due to its use as an argument when
// requesting the creation of a new backup.
type Origin struct {
	Environment string
	Machine     string
	Hostname    string
	Version     version.Number
}

// Metadata contains the metadata for a single state backup archive.
type Metadata struct {
	*filestorage.FileMetadata

	// Started records when the backup was started.
	Started time.Time
	// Finished records when the backup was complete.
	Finished *time.Time
	// Origin identifies where the backup was created.
	Origin Origin
	// Notes is an optional user-supplied annotation.
	Notes string
}

// NewMetadata returns a new Metadata for a state backup archive.  Only
// the start time and the version are set.
func NewMetadata() *Metadata {
	return &Metadata{
		FileMetadata: filestorage.NewMetadata(),
		Started:      time.Now().UTC(),
		Origin: Origin{
			Version: version.Current.Number,
		},
	}
}

// MarkComplete populates the remaining metadata values.  The default
// checksum format is used.
func (m *Metadata) MarkComplete(size int64, checksum string) error {
	if size == 0 {
		return errors.New("missing size")
	}
	if checksum == "" {
		return errors.New("missing checksum")
	}
	format := checksumFormat
	finished := time.Now().UTC()

	if err := m.SetFileInfo(size, checksum, format); err != nil {
		return errors.Annotate(err, "unexpected failure")
	}
	m.Finished = &finished

	return nil
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

// NewFromJSONBuffer extracts a new metadata from the JSON file.
func NewFromJSONBuffer(in io.Reader) (*Metadata, error) {
	var flat flatMetadata
	if err := json.NewDecoder(in).Decode(&flat); err != nil {
		return nil, errors.Trace(err)
	}

	var meta Metadata
	meta.SetID(flat.ID)

	err := meta.SetFile(flat.Size, flat.Checksum, flat.ChecksumFormat)
	if err != nil {
		return nil, errors.Trace(err)
	}

	if !flat.Stored.IsZero() {
		meta.SetStored(&flat.Stored)
	}

	meta.Started = flat.Started
	if !flat.Finished.IsZero() {
		meta.Finished = &flat.Finished
	}
	meta.Notes = flat.Notes
	meta.Origin = Origin{
		Environment: flat.Environment,
		Machine:     flat.Machine,
		Hostname:    flat.Hostname,
		Version:     flat.Version,
	}

	return &meta, nil
}
