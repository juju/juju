// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// backups contains all the stand-alone backup-related functionality for
// juju state.
package backups

import (
	"io"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/filestorage"

	"github.com/juju/juju/state/backups/db"
	"github.com/juju/juju/state/backups/files"
	"github.com/juju/juju/state/backups/metadata"
)

var logger = loggo.GetLogger("juju.state.backups")

var (
	getFilesToBackUp = files.GetFilesToBackUp
	getDBDumper      = db.NewDumper
	runCreate        = create
	finishMeta       = func(meta *metadata.Metadata, result *createResult) error {
		return meta.Finish(result.size, result.checksum, "", nil)
	}
	storeArchive = func(stor filestorage.FileStorage, meta *metadata.Metadata, file io.Reader) error {
		_, err := stor.Add(meta, file)
		return err
	}
)

// Backups is an abstraction around all juju backup-related functionality.
type Backups interface {

	// Create creates and stores a new juju backup archive and returns
	// its associated metadata.
	Create(paths files.Paths, dbInfo db.ConnInfo, origin metadata.Origin, notes string) (*metadata.Metadata, error)
	// Get returns the metadata and archive file associated with the ID.
	Get(id string) (*metadata.Metadata, io.ReadCloser, error)
	// List returns the metadata for all stored backups.
	List() ([]metadata.Metadata, error)
	// Remove deletes the backup from storage.
	Remove(id string) error
}

type backups struct {
	storage filestorage.FileStorage
}

// NewBackups returns a new Backups value using the provided DB info and
// file storage.
func NewBackups(stor filestorage.FileStorage) Backups {
	b := backups{
		storage: stor,
	}
	return &b
}

// Create creates and stores a new juju backup archive and returns
// its associated metadata.
func (b *backups) Create(paths files.Paths, dbInfo db.ConnInfo, origin metadata.Origin, notes string) (*metadata.Metadata, error) {

	// Prep the metadata.
	meta := metadata.NewMetadata(origin, notes, nil)
	metadataFile, err := meta.AsJSONBuffer() // ...unfinished.
	if err != nil {
		return nil, errors.Annotate(err, "while preparing the metadata")
	}

	// Create the archive.
	filesToBackUp, err := getFilesToBackUp("", paths)
	if err != nil {
		return nil, errors.Annotate(err, "while listing files to back up")
	}
	dumper := getDBDumper(dbInfo)
	args := createArgs{filesToBackUp, dumper, metadataFile}
	result, err := runCreate(&args)
	if err != nil {
		return nil, errors.Annotate(err, "while creating backup archive")
	}
	defer result.archiveFile.Close()

	// Finalize the metadata.
	err = finishMeta(meta, result)
	if err != nil {
		return nil, errors.Annotate(err, "while updating metadata")
	}

	// Store the archive.
	err = storeArchive(b.storage, meta, result.archiveFile)
	if err != nil {
		return nil, errors.Annotate(err, "while storing backup archive")
	}

	return meta, nil
}

// Get returns the metadata and archive file associated with the ID.
func (b *backups) Get(id string) (*metadata.Metadata, io.ReadCloser, error) {
	rawmeta, archiveFile, err := b.storage.Get(id)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}

	meta, ok := rawmeta.(*metadata.Metadata)
	if !ok {
		return nil, errors.New("did not get a backup.Metadata value from storage")
	}

	return meta, archiveFile, nil
}

// List returns the metadata for all stored backups.
func (b *backups) List() ([]metadata.Metadata, error) {
	metaList, err := b.storage.List()
	if err != nil {
		return nil, errors.Trace(err)
	}
	result := make([]metadata.Metadata, len(metaList))
	for i, meta := range metaList {
		m, ok := meta.(*metadata.Metadata)
		if !ok {
			return nil, errors.New("did not get a backup.Metadata value from storage")
		}
		result[i] = *m
	}
	return result, nil
}

// Remove deletes the backup from storage.
func (b *backups) Remove(id string) error {
	return errors.Trace(b.storage.Remove(id))
}
