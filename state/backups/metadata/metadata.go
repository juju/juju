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
		format = ChecksumFormat
	}
	if finished == nil {
		now := time.Now().UTC()
		finished = &now
	}

	if err := m.SetFile(size, checksum, format); err != nil {
		return errors.Annotate(err, "unexpected failure")
	}
	m.finished = finished

	return nil
}

type rawMetadata struct {
	ID             string
	Started        time.Time
	Finished       time.Time
	Checksum       string
	ChecksumFormat string
	Size           int64
	Stored         bool
	Notes          string
	Environment    string
	Machine        string
	Hostname       string
	Version        version.Number
}

// TODO(ericsnow) Move AsJSONBuffer to filestorage.Metadata.

// AsJSONBuffer returns a bytes.Buffer containing the JSON-ified metadata.
func (m *Metadata) AsJSONBuffer() (io.Reader, error) {
	origin := m.Origin()
	raw := rawMetadata{
		ID:             m.ID(),
		Started:        m.Started(),
		Checksum:       m.Checksum(),
		ChecksumFormat: m.ChecksumFormat(),
		Size:           m.Size(),
		Stored:         m.Stored(),
		Notes:          m.Notes(),
		Environment:    origin.Environment(),
		Machine:        origin.Machine(),
		Hostname:       origin.Hostname(),
		Version:        origin.Version(),
	}
	finished := m.Finished()
	if finished != nil {
		raw.Finished = *finished
	}
	data, err := json.Marshal(raw)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return bytes.NewBuffer(data), nil
}
