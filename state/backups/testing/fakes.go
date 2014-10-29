// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package testing

import (
	"io"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/filestorage"
	gc "gopkg.in/check.v1"

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
	return b.Error
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
	// Error holds the Metadata to return.
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
