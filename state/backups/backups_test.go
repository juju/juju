// Copyright 2013,2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"bytes"
	"io/ioutil"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/filestorage"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/state/backups"
	"github.com/juju/juju/state/backups/db"
	"github.com/juju/juju/state/backups/metadata"
	"github.com/juju/juju/testing"
)

type backupsSuite struct {
	testing.BaseSuite

	storage filestorage.FileStorage
	api     backups.Backups
}

var _ = gc.Suite(&backupsSuite{}) // Register the suite.

func (s *backupsSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	storage, err := filestorage.NewSimpleStorage(c.MkDir())
	c.Assert(err, gc.IsNil)
	s.storage = storage

	s.api = backups.NewBackups(s.storage)
}

func (s *backupsSuite) TestNewBackups(c *gc.C) {
	api := backups.NewBackups(s.storage)

	c.Check(api, gc.NotNil)
}

func (s *backupsSuite) TestCreateOkay(c *gc.C) {
	// Patch the internals.
	archiveFile := ioutil.NopCloser(bytes.NewBufferString("<compressed tarball>"))
	result := backups.NewTestCreateResult(archiveFile, 10, "<checksum>")
	received, testCreate := backups.NewTestCreate(result, nil)
	s.PatchValue(backups.RunCreate, testCreate)

	rootDir := "<was never set>"
	s.PatchValue(backups.GetFilesToBackUp, func(root string) ([]string, error) {
		rootDir = root
		return []string{"<some file>"}, nil
	})

	var receivedDBInfo *db.ConnInfo
	s.PatchValue(backups.GetDBDumper, func(info db.ConnInfo) db.Dumper {
		receivedDBInfo = &info
		return nil
	})

	// Run the backup.
	dbInfo := &db.ConnInfo{"a", "b", "c"}
	origin := metadata.NewOrigin("<env ID>", "<machine ID>", "<hostname>")
	meta, err := s.api.Create(dbInfo, origin, "some notes")

	// Test the call values.
	filesToBackUp, _ := backups.ExposeCreateArgs(received)
	c.Check(filesToBackUp, jc.SameContents, []string{"<some file>"})

	err = receivedDBInfo.Validate()
	c.Assert(err, gc.IsNil)
	c.Check(receivedDBInfo.Address, gc.Equals, "a")
	c.Check(receivedDBInfo.Username, gc.Equals, "b")
	c.Check(receivedDBInfo.Password, gc.Equals, "c")

	c.Check(rootDir, gc.Equals, "")

	// Check the resulting metadata.
	c.Check(meta.ID(), gc.Not(gc.Equals), "")
	c.Check(meta.Size(), gc.Equals, int64(10))
	c.Check(meta.Checksum(), gc.Equals, "<checksum>")
	c.Check(meta.Stored(), gc.Equals, true)
	metaOrigin := meta.Origin()
	c.Check(metaOrigin.Environment(), gc.Equals, "<env ID>")
	c.Check(metaOrigin.Machine(), gc.Equals, "<machine ID>")
	c.Check(metaOrigin.Hostname(), gc.Equals, "<hostname>")
	c.Check(meta.Notes(), gc.Equals, "some notes")

	// Check the file storage.
	storedMeta, storedFile, err := s.storage.Get(meta.ID())
	c.Check(err, gc.IsNil)
	c.Check(storedMeta, gc.DeepEquals, meta)
	data, err := ioutil.ReadAll(storedFile)
	c.Assert(err, gc.IsNil)
	c.Check(string(data), gc.Equals, "<compressed tarball>")
}
