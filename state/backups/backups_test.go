// Copyright 2013,2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"bytes"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"time" // Only used for time types.

	"github.com/juju/collections/set"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state/backups"
	backupstesting "github.com/juju/juju/state/backups/testing"
	"github.com/juju/juju/testing"
)

type backupsSuite struct {
	backupstesting.BaseSuite

	api backups.Backups
}

var _ = gc.Suite(&backupsSuite{}) // Register the suite.

func (s *backupsSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.api = backups.NewBackups(s.Storage)
}

func (s *backupsSuite) setStored(id string) *time.Time {
	s.Storage.ID = id
	s.Storage.Meta = backupstesting.NewMetadataStarted()
	s.Storage.Meta.SetID(id)
	stored := testing.NonZeroTime().UTC()
	s.Storage.Meta.SetStored(&stored)
	return &stored
}

type fakeDumper struct{}

func (*fakeDumper) Dump(dumpDir string) error {
	return nil
}

func (s *backupsSuite) checkFailure(c *gc.C, expected string) {
	s.PatchValue(backups.GetDBDumper, func(*backups.DBInfo) (backups.DBDumper, error) {
		return &fakeDumper{}, nil
	})

	paths := backups.Paths{DataDir: "/var/lib/juju"}
	targets := set.NewStrings("juju", "admin")
	dbInfo := backups.DBInfo{"a", "b", "c", targets}
	meta := backupstesting.NewMetadataStarted()
	meta.Notes = "some notes"

	_, err := s.api.Create(meta, &paths, &dbInfo, true, true)
	c.Check(err, gc.ErrorMatches, expected)
}

func (s *backupsSuite) TestNewBackups(c *gc.C) {
	api := backups.NewBackups(s.Storage)

	c.Check(api, gc.NotNil)
}

func (s *backupsSuite) TestCreateOkayKeepCopyNoDownload(c *gc.C) {
	s.testCreateOkay(c, true, true)
}

func (s *backupsSuite) TestCreateOkayKeepCopyFalse(c *gc.C) {
	s.testCreateOkay(c, false, true)
}

func (s *backupsSuite) TestCreateOkayNoDownloadFalse(c *gc.C) {
	s.testCreateOkay(c, true, false)
}

func (s *backupsSuite) testCreateOkay(c *gc.C, keepCopy, noDownload bool) {
	dataDir := c.MkDir()
	backupDir := c.MkDir()
	// Patch the internals.
	archiveFile := ioutil.NopCloser(bytes.NewBufferString("<compressed tarball>"))
	result := backups.NewTestCreateResult(
		archiveFile,
		10,
		"<checksum>",
		path.Join(backupDir, backups.TempFilename))
	received, testCreate := backups.NewTestCreate(result)
	s.PatchValue(backups.RunCreate, testCreate)

	rootDir := "<was never set>"
	s.PatchValue(backups.TestGetFilesToBackUp, func(root string, paths *backups.Paths, oldmachine string) ([]string, error) {
		rootDir = root
		return []string{"<some file>"}, nil
	})

	var receivedDBInfo *backups.DBInfo
	s.PatchValue(backups.GetDBDumper, func(info *backups.DBInfo) (backups.DBDumper, error) {
		receivedDBInfo = info
		return nil, nil
	})

	stored := s.setStored("spam")

	// Run the backup.
	paths := backups.Paths{BackupDir: backupDir, DataDir: dataDir}
	targets := set.NewStrings("juju", "admin")
	dbInfo := backups.DBInfo{"a", "b", "c", targets}
	meta := backupstesting.NewMetadataStarted()
	backupstesting.SetOrigin(meta, "<model ID>", "<machine ID>", "<hostname>")
	meta.Notes = "some notes"
	resultFilename, err := s.api.Create(meta, &paths, &dbInfo, keepCopy, noDownload)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(resultFilename, gc.Equals, path.Join(backupDir, backups.TempFilename))

	// Test the call values.
	if keepCopy {
		s.Storage.CheckCalled(c, "spam", meta, archiveFile, "Add", "Metadata")
	} else {
		c.Assert(s.Storage.Calls, jc.SameContents, []string{})
	}
	resultBackupDir, filesToBackUp, _ := backups.ExposeCreateArgs(received)
	c.Check(resultBackupDir, gc.Equals, backupDir)
	c.Check(filesToBackUp, jc.SameContents, []string{"<some file>"})

	c.Check(receivedDBInfo.Address, gc.Equals, "a")
	c.Check(receivedDBInfo.Username, gc.Equals, "b")
	c.Check(receivedDBInfo.Password, gc.Equals, "c")
	c.Check(receivedDBInfo.Targets, gc.DeepEquals, targets)

	c.Check(rootDir, gc.Equals, "")

	// Check the resulting metadata.
	if keepCopy {
		c.Check(meta, gc.Equals, s.Storage.MetaArg)
		c.Check(meta.ID(), gc.Equals, "spam")
		c.Check(meta.Stored().Unix(), gc.Equals, stored.Unix())
	}
	c.Check(meta.Size(), gc.Equals, int64(10))
	c.Check(meta.Checksum(), gc.Equals, "<checksum>")
	c.Check(meta.Origin.Model, gc.Equals, "<model ID>")
	c.Check(meta.Origin.Machine, gc.Equals, "<machine ID>")
	c.Check(meta.Origin.Hostname, gc.Equals, "<hostname>")
	c.Check(meta.Notes, gc.Equals, "some notes")

	// Check the file storage.
	if keepCopy {
		s.Storage.Meta = meta
		s.Storage.File = archiveFile
		storedMeta, storedFile, err := s.Storage.Get(meta.ID())
		c.Check(err, jc.ErrorIsNil)
		c.Check(storedMeta, gc.DeepEquals, meta)
		data, err := ioutil.ReadAll(storedFile)
		c.Assert(err, jc.ErrorIsNil)
		c.Check(string(data), gc.Equals, "<compressed tarball>")
	}
}

func (s *backupsSuite) TestCreateFailToListFiles(c *gc.C) {
	s.PatchValue(backups.TestGetFilesToBackUp, func(root string, paths *backups.Paths, oldmachine string) ([]string, error) {
		return nil, errors.New("failed!")
	})

	s.checkFailure(c, "while listing files to back up: failed!")
}

func (s *backupsSuite) TestCreateFailToCreate(c *gc.C) {
	s.PatchValue(backups.TestGetFilesToBackUp, func(root string, paths *backups.Paths, oldmachine string) ([]string, error) {
		return []string{}, nil
	})
	s.PatchValue(backups.RunCreate, backups.NewTestCreateFailure("failed!"))

	s.checkFailure(c, "while creating backup archive: failed!")
}

func (s *backupsSuite) TestCreateFailToFinishMeta(c *gc.C) {
	s.PatchValue(backups.TestGetFilesToBackUp, func(root string, paths *backups.Paths, oldmachine string) ([]string, error) {
		return []string{}, nil
	})
	_, testCreate := backups.NewTestCreate(nil)
	s.PatchValue(backups.RunCreate, testCreate)
	s.PatchValue(backups.FinishMeta, backups.NewTestMetaFinisher("failed!"))

	s.checkFailure(c, "while updating metadata: failed!")
}

func (s *backupsSuite) TestCreateFailToStoreArchive(c *gc.C) {
	s.PatchValue(backups.TestGetFilesToBackUp, func(root string, paths *backups.Paths, oldmachine string) ([]string, error) {
		return []string{}, nil
	})
	_, testCreate := backups.NewTestCreate(nil)
	s.PatchValue(backups.RunCreate, testCreate)
	s.PatchValue(backups.FinishMeta, backups.NewTestMetaFinisher(""))
	s.PatchValue(backups.StoreArchiveRef, backups.NewTestArchiveStorer("failed!"))

	s.checkFailure(c, "while storing backup archive: failed!")
}

func (s *backupsSuite) TestStoreArchive(c *gc.C) {
	stored := s.setStored("spam")

	meta := backupstesting.NewMetadataStarted()
	c.Assert(meta.ID(), gc.Equals, "")
	c.Assert(meta.Stored(), gc.IsNil)
	archive := &bytes.Buffer{}
	err := backups.StoreArchive(s.Storage, meta, archive)
	c.Assert(err, jc.ErrorIsNil)

	s.Storage.CheckCalled(c, "spam", meta, archive, "Add", "Metadata")
	c.Assert(meta.ID(), gc.Equals, "spam")
	c.Assert(meta.Stored(), jc.DeepEquals, stored)
}

func (s *backupsSuite) TestGetFileName(c *gc.C) {
	backupDir := c.MkDir()
	os.MkdirAll(backupDir, 0644)
	backupFilename := path.Join(backupDir, backups.TempFilename)
	backupFile, err := os.Create(backupFilename)
	c.Assert(err, jc.ErrorIsNil)
	backupFile.Write([]byte("archive file testing"))

	resultMeta, resultArchive, err := s.api.Get(backupFilename)
	c.Assert(err, jc.ErrorIsNil)
	defer resultArchive.Close()
	resultMeta.FileMetadata.Checksum()

	// Purpose for metadata here is for the checksum to be used by the
	// caller, so check it here.
	c.Assert(resultMeta.FileMetadata.Checksum(), gc.NotNil)
	b, err := ioutil.ReadAll(resultArchive)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(b), gc.Equals, "archive file testing")

	_, err = ioutil.ReadDir(backupDir)
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf("open %s: no such file or directory", backupDir))
}
