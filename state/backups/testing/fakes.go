// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"io"

	"github.com/juju/errors"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/state/backups"
	"github.com/juju/juju/state/backups/db"
	"github.com/juju/juju/state/backups/files"
	"github.com/juju/juju/state/backups/metadata"
)

// FakeBackups is an implementation of Backups to use for testing.
type FakeBackups struct {
	// Calls contains the order in which methods were called.
	Calls []string

	// Meta holds the Metadata to return.
	Meta *metadata.Metadata
	// MetaList holds the Metadata list to return.
	MetaList []metadata.Metadata
	// Archive holds the archive file to return.
	Archive io.ReadCloser
	// Error holds the Metadata to return.
	Error error

	// IDArg holds the ID that was passed in.
	IDArg string
	// PathsArg holds the Paths that was passed in.
	PathsArg *files.Paths
	// DBInfoArg holds the ConnInfo that was passed in.
	DBInfoArg *db.ConnInfo
	// OriginArg holds the Origin that was passed in.
	OriginArg *metadata.Origin
	// NotesArg holds the notes string that was passed in.
	NotesArg string

	// PrivateAddr holds the state server to be restored's address
	PrivateAddr string
	// InstanceId the id of the instance where we will restore
	InstanceId instance.Id
}

var _ backups.Backups = (*FakeBackups)(nil)

// Create creates and stores a new juju backup archive and returns
// its associated metadata.
func (b *FakeBackups) Create(paths files.Paths, dbInfo db.ConnInfo, origin metadata.Origin, notes string) (*metadata.Metadata, error) {
	b.Calls = append(b.Calls, "Create")

	b.PathsArg = &paths
	b.DBInfoArg = &dbInfo
	b.OriginArg = &origin
	b.NotesArg = notes

	return b.Meta, b.Error
}

// Get returns the metadata and archive file associated with the ID.
func (b *FakeBackups) Get(id string) (*metadata.Metadata, io.ReadCloser, error) {
	b.Calls = append(b.Calls, "Get")
	b.IDArg = id
	return b.Meta, b.Archive, b.Error
}

// List returns the metadata for all stored backups.
func (b *FakeBackups) List() ([]metadata.Metadata, error) {
	b.Calls = append(b.Calls, "List")
	return b.MetaList, b.Error
}

// Remove deletes the backup from storage.
func (b *FakeBackups) Remove(id string) error {
	b.Calls = append(b.Calls, "Remove")
	b.IDArg = id
	return errors.Trace(b.Error)
}

// Restore restores a machine to a backed up status.
func (b *FakeBackups) Restore(bkpFile io.ReadCloser, meta *metadata.Metadata, privateAddress string, newInstId instance.Id) error {
	b.Calls = append(b.Calls, "Restore")
	b.Meta = meta
	b.PrivateAddr = privateAddress
	b.InstanceId = newInstId
	return errors.Trace(b.Error)
}
