// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

// backups contains all the stand-alone backup-related functionality for
// juju state.
package backups

import (
	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils/filestorage"

	"github.com/juju/juju/state/backups/db"
	"github.com/juju/juju/state/backups/files"
	"github.com/juju/juju/state/backups/metadata"
)

var logger = loggo.GetLogger("juju.state.backups")

var (
	getFilesToBackUp                               = files.GetFilesToBackUp
	getDBDumper      (func(db.ConnInfo) db.Dumper) = db.NewDumper
	runCreate                                      = create
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
		return nil, errors.Annotate(err, "error listing files to back up")
	}
	dumper := getDBDumper(dbInfo)
	args := createArgs{filesToBackUp, dumper}
	result, err := runCreate(&args)
	if err != nil {
		return nil, errors.Annotate(err, "error creating backup archive")
	}
	defer result.archiveFile.Close()

	// Store the archive.
	err = meta.Finish(result.size, result.checksum, "", nil)
	if err != nil {
		return nil, errors.Annotate(err, "error updating metadata")
	}
	_, err = b.storage.Add(meta, result.archiveFile)
	if err != nil {
		return nil, errors.Annotate(err, "error storing backup archive")
	}

	return meta, nil
}
