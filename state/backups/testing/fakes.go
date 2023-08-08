// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"io"

	corebackups "github.com/juju/juju/core/backups"
	"github.com/juju/juju/core/instance"
	"github.com/juju/juju/state/backups"
)

// FakeBackups is an implementation of Backups to use for testing.
// TODO: (hml) 2018-04-25
// Let's change FakeBackups to using gomock or base.APICaller.
// Checking calls made and arguments is a pain.
type FakeBackups struct {
	// Calls contains the order in which methods were called.
	Calls []string

	// Meta holds the Metadata to return.
	Meta *corebackups.Metadata
	// MetaList holds the Metadata list to return.
	MetaList []*corebackups.Metadata
	// Archive holds the archive file to return.
	Archive io.ReadCloser
	// Error holds the error to return.
	Error error
	// Filename holds the name of the file to return.
	Filename string

	// IDArg holds the ID that was passed in.
	IDArg string
	// DBInfoArg holds the ConnInfo that was passed in.
	DBInfoArg *backups.DBInfo
	// MetaArg holds the backup metadata that was passed in.
	MetaArg *corebackups.Metadata
	// PrivateAddr Holds the address for the internal network of the machine.
	PrivateAddr string
	// InstanceId is the id of the machine to be restored.
	InstanceId instance.Id
	// ArchiveArg holds the backup archive that was passed in.
	ArchiveArg io.Reader
}

var _ backups.Backups = (*FakeBackups)(nil)

// Create creates and stores a new juju backup archive and returns
// its associated metadata.
func (b *FakeBackups) Create(
	meta *corebackups.Metadata,
	dbInfo *backups.DBInfo,
) (string, error) {
	b.Calls = append(b.Calls, "Create")

	b.DBInfoArg = dbInfo
	b.MetaArg = meta

	if b.Meta != nil {
		*meta = *b.Meta
	}

	return b.Filename, b.Error
}

// Get returns the metadata and archive file associated with the ID.
func (b *FakeBackups) Get(id string) (*corebackups.Metadata, io.ReadCloser, error) {
	b.Calls = append(b.Calls, "Get")
	b.IDArg = id
	return b.Meta, b.Archive, b.Error
}
