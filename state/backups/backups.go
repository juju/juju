// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

/*
Package backups contains all the stand-alone backup-related
functionality for juju state. That functionality is encapsulated by
the backups.Backups type. The package also exposes a few key helpers
and components.

Backups are not a part of juju state nor of normal state operations.
However, they certainly are tightly coupled with state (the very
subject of backups). This puts backups in an odd position, particularly
with regard to the storage of backup metadata and archives.

As noted above backups are about state but not a part of state. So
exposing backup-related methods on State would imply the wrong thing.
Thus most of the functionality here is defined at a high level without
relation to state. A few low-level parts or helpers are exposed as
functions to which you pass a state value. Those are kept to a minimum.

Note that state (and juju as a whole) currently does not have a
persistence layer abstraction to facilitate separating different
persistence needs and implementations. As a consequence, state's
data, whether about how an environment should look or about existing
resources within an environment, is dumped essentially straight into
State's mongo connection. The code in the state package does not
make any distinction between the two (nor does the package clearly
distinguish between state-related abstractions and state-related
data).

Backups add yet another category, merely taking advantage of State's
mongo for storage. In the interest of making the distinction clear,
among other reasons, backups uses its own database under state's mongo
connection.
*/
package backups

import (
	"io"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/filestorage"
)

const (
	// FilenamePrefix is the prefix used for backup archive files.
	FilenamePrefix = "juju-backup-"

	// FilenameTemplate is used with time.Time.Format to generate a filename.
	FilenameTemplate = FilenamePrefix + "20060102-150405.tar.gz"
)

var logger = loggo.GetLogger("juju.state.backups")

var (
	getFilesToBackUp = GetFilesToBackUp
	getDBDumper      = NewDBDumper
	runCreate        = create
	finishMeta       = func(meta *Metadata, result *createResult) error {
		return meta.MarkComplete(result.size, result.checksum)
	}
	storeArchive = StoreArchive
)

// StoreArchive sends the backup archive and its metadata to storage.
// It also sets the metadata's ID and Stored values.
func StoreArchive(stor filestorage.FileStorage, meta *Metadata, file io.Reader) error {
	id, err := stor.Add(meta, file)
	if err != nil {
		return errors.Trace(err)
	}
	meta.SetID(id)
	stored, err := stor.Metadata(id)
	if err != nil {
		return errors.Trace(err)
	}
	meta.SetStored(stored.Stored())
	return nil
}

// Backups is an abstraction around all juju backup-related functionality.
type Backups interface {
	// Create creates and stores a new juju backup archive. It updates
	// the provided metadata.
	Create(meta *Metadata, paths *Paths, dbInfo *DBInfo) error

	// Add stores the backup archive and returns its new ID.
	Add(archive io.Reader, meta *Metadata) (string, error)

	// Get returns the metadata and archive file associated with the ID.
	Get(id string) (*Metadata, io.ReadCloser, error)

	// List returns the metadata for all stored backups.
	List() ([]*Metadata, error)

	// Remove deletes the backup from storage.
	Remove(id string) error

	// Restore updates juju's state to the contents of the backup archive.
	Restore(backupId string, args RestoreArgs) error
}

type backups struct {
	storage filestorage.FileStorage
}

// NewBackups creates a new Backups value using the FileStorage provided.
func NewBackups(stor filestorage.FileStorage) Backups {
	b := backups{
		storage: stor,
	}
	return &b
}

// Create creates and stores a new juju backup archive and updates the
// provided metadata.
func (b *backups) Create(meta *Metadata, paths *Paths, dbInfo *DBInfo) error {
	meta.Started = time.Now().UTC()

	// The metadata file will not contain the ID or the "finished" data.
	// However, that information is not as critical. The alternatives
	// are either adding the metadata file to the archive after the fact
	// or adding placeholders here for the finished data and filling
	// them in afterward.  Neither is particularly trivial.
	metadataFile, err := meta.AsJSONBuffer()
	if err != nil {
		return errors.Annotate(err, "while preparing the metadata")
	}

	// Create the archive.
	filesToBackUp, err := getFilesToBackUp("", paths, meta.Origin.Machine)
	if err != nil {
		return errors.Annotate(err, "while listing files to back up")
	}
	dumper, err := getDBDumper(dbInfo)
	if err != nil {
		return errors.Annotate(err, "while preparing for DB dump")
	}
	args := createArgs{filesToBackUp, dumper, metadataFile}
	result, err := runCreate(&args)
	if err != nil {
		return errors.Annotate(err, "while creating backup archive")
	}
	defer result.archiveFile.Close()

	// Finalize the metadata.
	err = finishMeta(meta, result)
	if err != nil {
		return errors.Annotate(err, "while updating metadata")
	}

	// Store the archive.
	err = storeArchive(b.storage, meta, result.archiveFile)
	if err != nil {
		return errors.Annotate(err, "while storing backup archive")
	}

	return nil
}

// Add stores the backup archive and returns its new ID.
func (b *backups) Add(archive io.Reader, meta *Metadata) (string, error) {
	// Store the archive.
	err := storeArchive(b.storage, meta, archive)
	if err != nil {
		return "", errors.Annotate(err, "while storing backup archive")
	}

	return meta.ID(), nil
}

// Get retrieves the associated metadata and archive file from environment storage.
func (b *backups) Get(id string) (*Metadata, io.ReadCloser, error) {
	rawmeta, archiveFile, err := b.storage.Get(id)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	meta, ok := rawmeta.(*Metadata)
	if !ok {
		return nil, nil, errors.New("did not get a backups.Metadata value from storage")
	}

	return meta, archiveFile, nil
}

// List returns the metadata for all stored backups.
func (b *backups) List() ([]*Metadata, error) {
	metaList, err := b.storage.List()
	if err != nil {
		return nil, errors.Trace(err)
	}
	result := make([]*Metadata, len(metaList))
	for i, meta := range metaList {
		m, ok := meta.(*Metadata)
		if !ok {
			msg := "expected backups.Metadata value from storage for %q, got %T"
			return nil, errors.Errorf(msg, meta.ID(), meta)
		}
		result[i] = m
	}
	return result, nil
}

// Remove deletes the backup from storage.
func (b *backups) Remove(id string) error {
	return errors.Trace(b.storage.Remove(id))
}
