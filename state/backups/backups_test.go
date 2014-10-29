// Copyright 2013,2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"bytes"
	"io/ioutil"

	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state/backups"
	"github.com/juju/juju/state/backups/db"
	"github.com/juju/juju/state/backups/files"
	"github.com/juju/juju/state/backups/metadata"
	"github.com/juju/juju/testing"
)

type backupsSuite struct {
	testing.BaseSuite

	storage *fakeStorage
	api     backups.Backups
}

var _ = gc.Suite(&backupsSuite{}) // Register the suite.

func (s *backupsSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.storage = &fakeStorage{}
	s.api = backups.NewBackups(s.storage)
}

func (s *backupsSuite) checkFailure(c *gc.C, expected string) {
	paths := files.Paths{DataDir: "/var/lib/juju"}
	dbInfo := db.ConnInfo{"a", "b", "c"}
	origin := metadata.NewOrigin("<env ID>", "<machine ID>", "<hostname>")
	_, err := s.api.Create(paths, dbInfo, *origin, "some notes")

	c.Check(err, gc.ErrorMatches, expected)
}

func (s *backupsSuite) TestNewBackups(c *gc.C) {
	api := backups.NewBackups(s.storage)

	c.Check(api, gc.NotNil)
}

func (s *backupsSuite) TestCreateOkay(c *gc.C) {

	// Patch the internals.
	archiveFile := ioutil.NopCloser(bytes.NewBufferString("<compressed tarball>"))
	result := backups.NewTestCreateResult(archiveFile, 10, "<checksum>")
	received, testCreate := backups.NewTestCreate(result)
	s.PatchValue(backups.RunCreate, testCreate)

	rootDir := "<was never set>"
	s.PatchValue(backups.GetFilesToBackUp, func(root string, paths files.Paths) ([]string, error) {
		rootDir = root
		return []string{"<some file>"}, nil
	})

	var receivedDBInfo *db.ConnInfo
	s.PatchValue(backups.GetDBDumper, func(info db.ConnInfo) db.Dumper {
		receivedDBInfo = &info
		return nil
	})

	// Run the backup.
	paths := files.Paths{DataDir: "/var/lib/juju"}
	dbInfo := db.ConnInfo{"a", "b", "c"}
	origin := metadata.NewOrigin("<env ID>", "<machine ID>", "<hostname>")
	meta, err := s.api.Create(paths, dbInfo, *origin, "some notes")

	// Test the call values.
	s.storage.check(c, "", meta, archiveFile, "Add")
	filesToBackUp, _ := backups.ExposeCreateArgs(received)
	c.Check(filesToBackUp, jc.SameContents, []string{"<some file>"})

	err = receivedDBInfo.Validate()
	c.Assert(err, gc.IsNil)
	c.Check(receivedDBInfo.Address, gc.Equals, "a")
	c.Check(receivedDBInfo.Username, gc.Equals, "b")
	c.Check(receivedDBInfo.Password, gc.Equals, "c")

	c.Check(rootDir, gc.Equals, "")

	// Check the resulting metadata.
	c.Check(meta, gc.Equals, s.storage.metaArg)
	c.Check(meta.Size(), gc.Equals, int64(10))
	c.Check(meta.Checksum(), gc.Equals, "<checksum>")
	c.Check(meta.Stored(), gc.NotNil)
	c.Check(meta.Origin.Environment, gc.Equals, "<env ID>")
	c.Check(meta.Origin.Machine, gc.Equals, "<machine ID>")
	c.Check(meta.Origin.Hostname, gc.Equals, "<hostname>")
	c.Check(meta.Notes, gc.Equals, "some notes")

	// Check the file storage.
	s.storage.meta = meta
	s.storage.file = archiveFile
	storedMeta, storedFile, err := s.storage.Get(meta.ID())
	c.Check(err, gc.IsNil)
	c.Check(storedMeta, gc.DeepEquals, meta)
	data, err := ioutil.ReadAll(storedFile)
	c.Assert(err, gc.IsNil)
	c.Check(string(data), gc.Equals, "<compressed tarball>")
}

func (s *backupsSuite) TestCreateFailToListFiles(c *gc.C) {
	s.PatchValue(backups.GetFilesToBackUp, func(root string, paths files.Paths) ([]string, error) {
		return nil, errors.New("failed!")
	})

	s.checkFailure(c, "while listing files to back up: failed!")
}

func (s *backupsSuite) TestCreateFailToCreate(c *gc.C) {
	s.PatchValue(backups.GetFilesToBackUp, func(root string, paths files.Paths) ([]string, error) {
		return []string{}, nil
	})
	s.PatchValue(backups.RunCreate, backups.NewTestCreateFailure("failed!"))

	s.checkFailure(c, "while creating backup archive: failed!")
}

func (s *backupsSuite) TestCreateFailToFinishMeta(c *gc.C) {
	s.PatchValue(backups.GetFilesToBackUp, func(root string, paths files.Paths) ([]string, error) {
		return []string{}, nil
	})
	_, testCreate := backups.NewTestCreate(nil)
	s.PatchValue(backups.RunCreate, testCreate)
	s.PatchValue(backups.FinishMeta, backups.NewTestMetaFinisher("failed!"))

	s.checkFailure(c, "while updating metadata: failed!")
}

func (s *backupsSuite) TestCreateFailToStoreArchive(c *gc.C) {
	s.PatchValue(backups.GetFilesToBackUp, func(root string, paths files.Paths) ([]string, error) {
		return []string{}, nil
	})
	_, testCreate := backups.NewTestCreate(nil)
	s.PatchValue(backups.RunCreate, testCreate)
	s.PatchValue(backups.FinishMeta, backups.NewTestMetaFinisher(""))
	s.PatchValue(backups.StoreArchive, backups.NewTestArchiveStorer("failed!"))

	s.checkFailure(c, "while storing backup archive: failed!")
}
