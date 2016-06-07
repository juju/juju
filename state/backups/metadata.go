// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"bytes"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"io"
	"os"
	"time"

	"github.com/juju/errors"
	jujuversion "github.com/juju/juju/version"
	"github.com/juju/utils/filestorage"
	"github.com/juju/version"
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
	Model    string
	Machine  string
	Hostname string
	Version  version.Number
	Series   string
}

// UnknownString is a marker value for string fields with unknown values.
const UnknownString = "<unknown>"

// UnknownVersion is a marker value for version fields with unknown values.
var UnknownVersion = version.MustParse("9999.9999.9999")

// UnknownOrigin returns a new backups origin with unknown values.
func UnknownOrigin() Origin {
	return Origin{
		Model:    UnknownString,
		Machine:  UnknownString,
		Hostname: UnknownString,
		Version:  UnknownVersion,
	}
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

	// TODO(wallyworld) - remove these ASAP
	// These are only used by the restore CLI when re-bootstrapping.
	// We will use a better solution but the way restore currently
	// works, we need them and they are no longer available via
	// bootstrap config. We will need to ifx how re-bootstrap deals
	// with these keys to address the issue.

	// CACert is the controller CA certificate.
	CACert string

	// CAPrivateKey is the controller CA private key.
	CAPrivateKey string
}

// NewMetadata returns a new Metadata for a state backup archive.  Only
// the start time and the version are set.
func NewMetadata() *Metadata {
	return &Metadata{
		FileMetadata: filestorage.NewMetadata(),
		// TODO(fwereade): 2016-03-17 lp:1558657
		Started: time.Now().UTC(),
		Origin: Origin{
			Version: jujuversion.Current,
		},
	}
}

// NewMetadataState composes a new backup metadata with its origin
// values set.  The model UUID comes from state.  The hostname is
// retrieved from the OS.
func NewMetadataState(db DB, machine, series string) (*Metadata, error) {
	// hostname could be derived from the model...
	hostname, err := os.Hostname()
	if err != nil {
		// If os.Hostname() is not working, something is woefully wrong.
		// Run for the hills.
		return nil, errors.Annotate(err, "could not get hostname (system unstable?)")
	}

	meta := NewMetadata()
	meta.Origin.Model = db.ModelTag().Id()
	meta.Origin.Machine = machine
	meta.Origin.Hostname = hostname
	meta.Origin.Series = series

	si, err := db.StateServingInfo()
	if err != nil {
		return nil, errors.Annotate(err, "could not get server secrets")
	}
	cfg, err := db.ModelConfig()
	if err != nil {
		return nil, errors.Annotate(err, "could not get model config")
	}
	meta.CACert, _ = cfg.CACert()
	meta.CAPrivateKey = si.CAPrivateKey
	return meta, nil
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
	// TODO(fwereade): 2016-03-17 lp:1558657
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
	Series      string

	CACert       string
	CAPrivateKey string
}

// TODO(ericsnow) Move AsJSONBuffer to filestorage.Metadata.

// AsJSONBuffer returns a bytes.Buffer containing the JSON-ified metadata.
func (m *Metadata) AsJSONBuffer() (io.Reader, error) {
	flat := flatMetadata{
		ID: m.ID(),

		Checksum:       m.Checksum(),
		ChecksumFormat: m.ChecksumFormat(),
		Size:           m.Size(),

		Started:      m.Started,
		Notes:        m.Notes,
		Environment:  m.Origin.Model,
		Machine:      m.Origin.Machine,
		Hostname:     m.Origin.Hostname,
		Version:      m.Origin.Version,
		Series:       m.Origin.Series,
		CACert:       m.CACert,
		CAPrivateKey: m.CAPrivateKey,
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

// NewMetadataJSONReader extracts a new metadata from the JSON file.
func NewMetadataJSONReader(in io.Reader) (*Metadata, error) {
	var flat flatMetadata
	if err := json.NewDecoder(in).Decode(&flat); err != nil {
		return nil, errors.Trace(err)
	}

	meta := NewMetadata()
	meta.SetID(flat.ID)

	err := meta.SetFileInfo(flat.Size, flat.Checksum, flat.ChecksumFormat)
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
		Model:    flat.Environment,
		Machine:  flat.Machine,
		Hostname: flat.Hostname,
		Version:  flat.Version,
		Series:   flat.Series,
	}

	// TODO(wallyworld) - put these in a separate file.
	meta.CACert = flat.CACert
	meta.CAPrivateKey = flat.CAPrivateKey

	return meta, nil
}

func fileTimestamp(fi os.FileInfo) time.Time {
	timestamp := creationTime(fi)
	if !timestamp.IsZero() {
		return timestamp
	}
	// Fall back to modification time.
	return fi.ModTime()
}

// BuildMetadata generates the metadata for a backup archive file.
func BuildMetadata(file *os.File) (*Metadata, error) {

	// Extract the file size.
	fi, err := file.Stat()
	if err != nil {
		return nil, errors.Trace(err)
	}
	size := fi.Size()

	// Extract the timestamp.
	timestamp := fileTimestamp(fi)

	// Get the checksum.
	hasher := sha1.New()
	_, err = io.Copy(hasher, file)
	if err != nil {
		return nil, errors.Trace(err)
	}
	rawsum := hasher.Sum(nil)
	checksum := base64.StdEncoding.EncodeToString(rawsum)

	// Build the metadata.
	meta := NewMetadata()
	meta.Started = time.Time{}
	meta.Origin = UnknownOrigin()
	err = meta.MarkComplete(size, checksum)
	if err != nil {
		return nil, errors.Trace(err)
	}
	meta.Finished = &timestamp
	return meta, nil
}
