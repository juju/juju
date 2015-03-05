// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"io"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/filestorage"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/instance"
	"github.com/juju/juju/state/backups"
)

// FakeBackups is an implementation of Backups to use for testing.
type FakeBackups struct {
	// Calls contains the order in which methods were called.
	Calls []string

	// Meta holds the Metadata to return.
	Meta *backups.Metadata
	// MetaList holds the Metadata list to return.
	MetaList []*backups.Metadata
	// Archive holds the archive file to return.
	Archive io.ReadCloser
	// Error holds the error to return.
	Error error

	// IDArg holds the ID that was passed in.
	IDArg string
	// PathsArg holds the Paths that was passed in.
	PathsArg *backups.Paths
	// DBInfoArg holds the ConnInfo that was passed in.
	DBInfoArg *backups.DBInfo
	// MetaArg holds the backup metadata that was passed in.
	MetaArg *backups.Metadata
	// PrivateAddr Holds the address for the internal network of the machine.
	PrivateAddr string
	// InstanceId Is the id of the machine to be restored.
	InstanceId instance.Id
	// ArchiveArg holds the backup archive that was passed in.
	ArchiveArg io.Reader
}

var _ backups.Backups = (*FakeBackups)(nil)

// Create creates and stores a new juju backup archive and returns
// its associated metadata.
func (b *FakeBackups) Create(meta *backups.Metadata, paths *backups.Paths, dbInfo *backups.DBInfo) error {
	b.Calls = append(b.Calls, "Create")

	b.PathsArg = paths
	b.DBInfoArg = dbInfo
	b.MetaArg = meta

	if b.Meta != nil {
		*meta = *b.Meta
	}

	return b.Error
}

// Add stores the backup and returns its new ID.
func (b *FakeBackups) Add(archive io.Reader, meta *backups.Metadata) (string, error) {
	b.Calls = append(b.Calls, "Add")
	b.ArchiveArg = archive
	b.MetaArg = meta
	id := ""
	if b.Meta != nil {
		id = b.Meta.ID()
	}
	return id, b.Error
}

// Get returns the metadata and archive file associated with the ID.
func (b *FakeBackups) Get(id string) (*backups.Metadata, io.ReadCloser, error) {
	b.Calls = append(b.Calls, "Get")
	b.IDArg = id
	return b.Meta, b.Archive, b.Error
}

// List returns the metadata for all stored backups.
func (b *FakeBackups) List() ([]*backups.Metadata, error) {
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
func (b *FakeBackups) Restore(bkpId string, args backups.RestoreArgs) error {
	b.Calls = append(b.Calls, "Restore")
	b.PrivateAddr = args.PrivateAddress
	b.InstanceId = args.NewInstId
	return errors.Trace(b.Error)
}

// TODO(ericsnow) FakeStorage should probably move over to the utils repo.

// FakeStorage is a FileStorage implementation to use when testing
// backups.
type FakeStorage struct {
	// Calls contains the order in which methods were called.
	Calls []string

	// ID is the stored backup ID to return.
	ID string
	// Meta holds the Metadata to return.
	Meta filestorage.Metadata
	// MetaList holds the Metadata list to return.
	MetaList []filestorage.Metadata
	// File holds the stored file to return.
	File io.ReadCloser
	// Error holds the error to return.
	Error error

	// IDArg holds the ID that was passed in.
	IDArg string
	// MetaArg holds the Metadata that was passed in.
	MetaArg filestorage.Metadata
	// FileArg holds the file that was passed in.
	FileArg io.Reader
}

// CheckCalled verifies that the fake was called as expected.
func (s *FakeStorage) CheckCalled(c *gc.C, id string, meta filestorage.Metadata, file io.Reader, calls ...string) {
	c.Check(s.Calls, jc.DeepEquals, calls)
	c.Check(s.IDArg, gc.Equals, id)
	c.Check(s.MetaArg, gc.Equals, meta)
	c.Check(s.FileArg, gc.Equals, file)
}

func (s *FakeStorage) Metadata(id string) (filestorage.Metadata, error) {
	s.Calls = append(s.Calls, "Metadata")
	s.IDArg = id
	return s.Meta, s.Error
}

func (s *FakeStorage) Get(id string) (filestorage.Metadata, io.ReadCloser, error) {
	s.Calls = append(s.Calls, "Get")
	s.IDArg = id
	return s.Meta, s.File, s.Error
}

func (s *FakeStorage) List() ([]filestorage.Metadata, error) {
	s.Calls = append(s.Calls, "List")
	return s.MetaList, s.Error
}

func (s *FakeStorage) Add(meta filestorage.Metadata, file io.Reader) (string, error) {
	s.Calls = append(s.Calls, "Add")
	s.MetaArg = meta
	s.FileArg = file
	return s.ID, s.Error
}

func (s *FakeStorage) SetFile(id string, file io.Reader) error {
	s.Calls = append(s.Calls, "SetFile")
	s.IDArg = id
	s.FileArg = file
	return s.Error
}

func (s *FakeStorage) Remove(id string) error {
	s.Calls = append(s.Calls, "Remove")
	s.IDArg = id
	return s.Error
}

func (s *FakeStorage) Close() error {
	s.Calls = append(s.Calls, "Close")
	return s.Error
}
