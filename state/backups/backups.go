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
	Create(dbInfo db.ConnInfo, origin metadata.Origin, notes string) (*metadata.Metadata, error)
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
func (b *backups) Create(dbInfo db.ConnInfo, origin metadata.Origin, notes string) (*metadata.Metadata, error) {

	// Prep the metadata.
	meta := metadata.NewMetadata(origin, notes, nil)

	// Create the archive.
	filesToBackUp, err := getFilesToBackUp("")
	if err != nil {
		return nil, errors.Annotate(err, "while listing files to back up")
	}
	dumper := getDBDumper(dbInfo)
	args := createArgs{filesToBackUp, dumper}
	result, err := runCreate(&args)
	if err != nil {
		return nil, errors.Annotate(err, "while creating backup archive")
	}
	defer result.archiveFile.Close()

	// Store the archive.
	err = finishMeta(meta, result)
	if err != nil {
		return nil, errors.Annotate(err, "while updating metadata")
	}
	err = storeArchive(b.storage, meta, result.archiveFile)
	if err != nil {
		return nil, errors.Annotate(err, "while storing backup archive")
	}

	return meta, nil
}
