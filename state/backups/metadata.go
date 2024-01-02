// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups

import (
	"bytes"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"io"
	"math"
	"os"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	"github.com/juju/utils/v3/filestorage"
	"github.com/juju/version/v2"

	"github.com/juju/juju/controller"
	jujuversion "github.com/juju/juju/version"
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
	Base     string
}

// UnknownString is a marker value for string fields with unknown values.
const UnknownString = "<unknown>"

// UnknownVersion is a marker value for version fields with unknown values.
var UnknownVersion = version.MustParse("9999.9999.9999")

// UnknownInt64 is a marker value for int64 fields with unknown values.
var UnknownInt64 = int64(math.MaxInt64)

// UnknownOrigin returns a new backups origin with unknown values.
func UnknownOrigin() Origin {
	return Origin{
		Model:    UnknownString,
		Machine:  UnknownString,
		Hostname: UnknownString,
		Version:  UnknownVersion,
	}
}

// UnknownController returns a new backups origin with unknown values.
func UnknownController() ControllerMetadata {
	return ControllerMetadata{
		UUID:              UnknownString,
		MachineID:         UnknownString,
		MachineInstanceID: UnknownString,
		HANodes:           UnknownInt64,
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

	// FormatVersion stores format version of these metadata.
	FormatVersion int64

	// Controller contains metadata about the controller where the backup was taken.
	Controller ControllerMetadata
}

// ControllerMetadata contains controller specific metadata.
type ControllerMetadata struct {
	// UUID contains the controller UUID.
	UUID string

	// MachineID contains controller machine id from which this backup is taken.
	MachineID string

	// MachineInstanceID contains controller machine's instance id from which this backup is taken.
	MachineInstanceID string

	// HANodes contains the number of nodes in this controller's HA configuration.
	HANodes int64
}

// All un-versioned metadata is considered to be version 0,
// so the versions start with 1.
const currentFormatVersion = 1

// NewMetadata returns a new Metadata for a state backup archive,
// in the most current format.
func NewMetadata() *Metadata {
	return &Metadata{
		FileMetadata: filestorage.NewMetadata(),
		// TODO(fwereade): 2016-03-17 lp:1558657
		Started: time.Now().UTC(),
		Origin: Origin{
			Version: jujuversion.Current,
		},
		FormatVersion: currentFormatVersion,
		Controller:    ControllerMetadata{},
	}
}

type backend interface {
	ModelTag() names.ModelTag
	ControllerConfig() (controller.Config, error)
	StateServingInfo() (controller.StateServingInfo, error)
}

// NewMetadataState composes a new backup metadata based on the current Juju state.
func NewMetadataState(db backend, machine, base string) (*Metadata, error) {
	hostname, err := os.Hostname()
	if err != nil {
		// If os.Hostname() is not working, something is woefully wrong.
		return nil, errors.Annotate(err, "could not get hostname (system unstable?)")
	}

	meta := NewMetadata()
	meta.Origin.Model = db.ModelTag().Id()
	meta.Origin.Machine = machine
	meta.Origin.Hostname = hostname
	meta.Origin.Base = base

	controllerCfg, err := db.ControllerConfig()
	if err != nil {
		return nil, errors.Annotate(err, "could not get controller config")
	}
	meta.Controller.UUID = controllerCfg.ControllerUUID()
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

// flatMetadataV0 contains old, un-versioned format of backup, aka v0.
type flatMetadataV0 struct {
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
	Base        string

	CACert       string
	CAPrivateKey string
}

func (flat *flatMetadataV0) inflate() (*Metadata, error) {
	meta := NewMetadata()
	meta.SetID(flat.ID)
	meta.FormatVersion = 0
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
		Base:     flat.Base,
	}
	return meta, nil
}

// flatMetadata contains the latest format of the backup.
// NOTE If any changes need to be made here, rename this struct to
// reflect version 1, for example flatMetadataV1 and construct
// new flatMetadata with desired modifications.
type flatMetadata struct {
	ID            string
	FormatVersion int64

	// file storage

	Checksum       string
	ChecksumFormat string
	Size           int64
	Stored         time.Time

	// backup

	Started                     time.Time
	Finished                    time.Time
	Notes                       string
	ModelUUID                   string
	Machine                     string
	Hostname                    string
	Version                     version.Number
	Base                        string
	ControllerUUID              string
	HANodes                     int64
	ControllerMachineID         string
	ControllerMachineInstanceID string
}

func (m *Metadata) flat() flatMetadata {
	flat := flatMetadata{
		ID:                          m.ID(),
		Checksum:                    m.Checksum(),
		ChecksumFormat:              m.ChecksumFormat(),
		Size:                        m.Size(),
		Started:                     m.Started,
		Notes:                       m.Notes,
		ModelUUID:                   m.Origin.Model,
		Machine:                     m.Origin.Machine,
		Hostname:                    m.Origin.Hostname,
		Version:                     m.Origin.Version,
		Base:                        m.Origin.Base,
		FormatVersion:               m.FormatVersion,
		ControllerUUID:              m.Controller.UUID,
		ControllerMachineID:         m.Controller.MachineID,
		ControllerMachineInstanceID: m.Controller.MachineInstanceID,
		HANodes:                     m.Controller.HANodes,
	}
	stored := m.Stored()
	if stored != nil {
		flat.Stored = *stored
	}

	if m.Finished != nil {
		flat.Finished = *m.Finished
	}
	return flat
}

func (flat *flatMetadata) inflate() (*Metadata, error) {
	meta := NewMetadata()
	meta.SetID(flat.ID)
	meta.FormatVersion = flat.FormatVersion

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
		Model:    flat.ModelUUID,
		Machine:  flat.Machine,
		Hostname: flat.Hostname,
		Version:  flat.Version,
		Base:     flat.Base,
	}

	meta.Controller = ControllerMetadata{
		UUID:              flat.ControllerUUID,
		MachineID:         flat.ControllerMachineID,
		MachineInstanceID: flat.ControllerMachineInstanceID,
		HANodes:           flat.HANodes,
	}
	return meta, nil
}

// AsJSONBuffer returns a bytes.Buffer containing the JSON-ified metadata.
// This will always produce latest known format.
func (m *Metadata) AsJSONBuffer() (io.Reader, error) {
	var outfile bytes.Buffer
	if err := json.NewEncoder(&outfile).Encode(m.flat()); err != nil {
		return nil, errors.Trace(err)
	}
	return &outfile, nil
}

// NewMetadataJSONReader extracts a new metadata from the JSON file.
func NewMetadataJSONReader(in io.Reader) (*Metadata, error) {
	data, err := io.ReadAll(in)
	if err != nil {
		return nil, errors.Trace(err)
	}
	// We always want to decode into the most recent format version.
	var flat flatMetadata
	if err := json.Unmarshal(data, &flat); err != nil {
		return nil, errors.Trace(err)
	}

	// Cater for old backup files, taken as version 0 or with no version.
	switch flat.FormatVersion {
	case 0:
		{
			var v0 flatMetadataV0
			if err := json.Unmarshal(data, &v0); err != nil {
				return nil, errors.Trace(err)
			}
			return v0.inflate()
		}
	case 1:
		return flat.inflate()
	default:
		return nil, errors.NotSupportedf("backup format %d", flat.FormatVersion)
	}
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
	meta.FormatVersion = UnknownInt64
	meta.Controller = UnknownController()
	err = meta.MarkComplete(size, checksum)
	if err != nil {
		return nil, errors.Trace(err)
	}
	meta.Finished = &timestamp
	return meta, nil
}
