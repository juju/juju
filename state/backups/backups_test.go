// Copyright 2013,2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"

	"github.com/dustin/go-humanize"
	"github.com/juju/collections/set"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	corebackups "github.com/juju/juju/core/backups"
	corebackupstesting "github.com/juju/juju/core/backups/testing"
	"github.com/juju/juju/state/backups"
	backupstesting "github.com/juju/juju/state/backups/testing"
)

type backupsSuite struct {
	backupstesting.BaseSuite

	paths *corebackups.Paths
	api   backups.Backups

	totalDiskMiB     uint64
	availableDiskMiB uint64
	dirSizeBytes     int64
	dbSizeMiB        int
}

var _ = gc.Suite(&backupsSuite{}) // Register the suite.

func (s *backupsSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	s.paths = &corebackups.Paths{
		BackupDir: c.MkDir(),
		DataDir:   c.MkDir(),
	}
	s.api = backups.NewBackups(s.paths)
	s.PatchValue(backups.AvailableDisk, func(string) uint64 {
		return s.availableDiskMiB
	})
	s.PatchValue(backups.TotalDisk, func(string) uint64 {
		return s.totalDiskMiB
	})
	s.PatchValue(backups.DirSize, func(path string) (int64, error) {
		return s.dirSizeBytes, nil
	})
}

type fakeDumper struct{}

func (*fakeDumper) Dump(dumpDir string) error {
	return nil
}

func (*fakeDumper) IsSnap() bool {
	return false
}

func (s *backupsSuite) checkFailure(c *gc.C, expected string) {
	s.PatchValue(backups.GetDBDumper, func(*backups.DBInfo) (backups.DBDumper, error) {
		return &fakeDumper{}, nil
	})

	targets := set.NewStrings("juju", "admin")
	dbInfo := backups.DBInfo{
		Address: "a", Username: "b", Password: "c",
		Targets:      targets,
		ApproxSizeMB: s.dbSizeMiB}
	meta := corebackupstesting.NewMetadataStarted()
	meta.Notes = "some notes"

	_, err := s.api.Create(meta, &dbInfo)
	c.Check(err, gc.ErrorMatches, expected)
}

func (s *backupsSuite) TestCreateOkay(c *gc.C) {
	// Patch the internals.
	archiveFile := io.NopCloser(bytes.NewBufferString("<compressed tarball>"))
	result := backups.NewTestCreateResult(
		archiveFile,
		10,
		"<checksum>",
		path.Join(s.paths.BackupDir, "test-backup.tar.gz"))
	received, testCreate := backups.NewTestCreate(result)
	s.PatchValue(backups.RunCreate, testCreate)

	rootDir := "<was never set>"
	s.PatchValue(backups.TestGetFilesToBackUp, func(root string, paths *corebackups.Paths) ([]string, error) {
		rootDir = root
		return []string{"<some file>"}, nil
	})

	var receivedDBInfo *backups.DBInfo
	s.PatchValue(backups.GetDBDumper, func(info *backups.DBInfo) (backups.DBDumper, error) {
		receivedDBInfo = info
		return nil, nil
	})

	// Run the backup.
	targets := set.NewStrings("juju", "admin")
	dbInfo := backups.DBInfo{
		Address: "a", Username: "b", Password: "c",
		Targets:      targets,
		ApproxSizeMB: s.dbSizeMiB}
	meta := corebackupstesting.NewMetadataStarted()
	corebackupstesting.SetOrigin(meta, "<model ID>", "<machine ID>", "<hostname>")
	meta.Notes = "some notes"
	resultFilename, err := s.api.Create(meta, &dbInfo)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(resultFilename, gc.Equals, path.Join(s.paths.BackupDir, "test-backup.tar.gz"))

	// Test the call values.
	resultBackupDir, filesToBackUp, _ := backups.ExposeCreateArgs(received)
	c.Check(resultBackupDir, gc.Equals, s.paths.BackupDir)
	c.Check(filesToBackUp, jc.SameContents, []string{"<some file>"})

	c.Check(receivedDBInfo.Address, gc.Equals, "a")
	c.Check(receivedDBInfo.Username, gc.Equals, "b")
	c.Check(receivedDBInfo.Password, gc.Equals, "c")
	c.Check(receivedDBInfo.Targets, gc.DeepEquals, targets)

	c.Check(rootDir, gc.Equals, "")

	// Check the resulting metadata.
	c.Check(meta.Size(), gc.Equals, int64(10))
	c.Check(meta.Checksum(), gc.Equals, "<checksum>")
	c.Check(meta.Origin.Model, gc.Equals, "<model ID>")
	c.Check(meta.Origin.Machine, gc.Equals, "<machine ID>")
	c.Check(meta.Origin.Hostname, gc.Equals, "<hostname>")
	c.Check(meta.Notes, gc.Equals, "some notes")
}

func (s *backupsSuite) TestCreateFailToListFiles(c *gc.C) {
	s.PatchValue(backups.TestGetFilesToBackUp, func(root string, paths *corebackups.Paths) ([]string, error) {
		return nil, errors.New("failed!")
	})

	s.checkFailure(c, "while listing files to back up: failed!")
}

func (s *backupsSuite) TestCreateFailToCreate(c *gc.C) {
	s.PatchValue(backups.TestGetFilesToBackUp, func(root string, paths *corebackups.Paths) ([]string, error) {
		return []string{}, nil
	})
	s.PatchValue(backups.RunCreate, backups.NewTestCreateFailure("failed!"))

	s.checkFailure(c, "while creating backup archive: failed!")
}

func (s *backupsSuite) TestCreateFailToFinishMeta(c *gc.C) {
	s.PatchValue(backups.TestGetFilesToBackUp, func(root string, paths *corebackups.Paths) ([]string, error) {
		return []string{}, nil
	})
	_, testCreate := backups.NewTestCreate(nil)
	s.PatchValue(backups.RunCreate, testCreate)
	s.PatchValue(backups.FinishMeta, backups.NewTestMetaFinisher("failed!"))

	s.checkFailure(c, "while updating metadata: failed!")
}

func (s *backupsSuite) TestNotEnoughDiskSpaceSmallBackup(c *gc.C) {
	s.PatchValue(backups.TestGetFilesToBackUp, func(root string, paths *corebackups.Paths) ([]string, error) {
		return []string{"file1"}, nil
	})
	s.dbSizeMiB = 6
	s.dirSizeBytes = 3 * humanize.MiByte
	s.availableDiskMiB = 10 * humanize.MiByte
	s.totalDiskMiB = 200 * humanize.GiByte

	s.checkFailure(c, "not enough free space in .*; want 5129MiB, have 10MiB")
}

func (s *backupsSuite) TestNotEnoughDiskSpaceLargeBackup(c *gc.C) {
	s.PatchValue(backups.TestGetFilesToBackUp, func(root string, paths *corebackups.Paths) ([]string, error) {
		return []string{"file1"}, nil
	})
	s.dbSizeMiB = 100
	s.dirSizeBytes = 50 * humanize.GiByte
	s.availableDiskMiB = 10 * humanize.MiByte
	s.totalDiskMiB = 200 * humanize.GiByte

	s.checkFailure(c, "not enough free space in .*; want 61560MiB, have 10MiB")
}

func (s *backupsSuite) TestNotEnoughDiskSpaceSmallDisk(c *gc.C) {
	s.PatchValue(backups.TestGetFilesToBackUp, func(root string, paths *corebackups.Paths) ([]string, error) {
		return []string{"file1"}, nil
	})
	s.dbSizeMiB = 6
	s.dirSizeBytes = 3 * humanize.MiByte
	s.availableDiskMiB = 10 * humanize.MiByte
	s.totalDiskMiB = 20 * humanize.GiByte

	s.checkFailure(c, "not enough free space in .*; want 2057MiB, have 10MiB")
}

func (s *backupsSuite) TestGetFileName(c *gc.C) {
	backupSubDir := filepath.Join(s.paths.BackupDir, "a", "b")
	err := os.MkdirAll(backupSubDir, 0755)
	c.Assert(err, jc.ErrorIsNil)
	backupFilename := path.Join(backupSubDir, "juju-backup-123.tar.gz")
	backupFile, err := os.Create(backupFilename)
	c.Assert(err, jc.ErrorIsNil)
	_, err = backupFile.Write([]byte("archive file testing"))
	c.Assert(err, jc.ErrorIsNil)

	_, _, err = s.api.Get("/etc/hostname")
	c.Assert(err, gc.ErrorMatches, `backup file "/etc/hostname" not valid`)

	resultMeta, resultArchive, err := s.api.Get(backupFilename)
	c.Assert(err, jc.ErrorIsNil)
	defer resultArchive.Close()
	resultMeta.FileMetadata.Checksum()

	// Purpose for metadata here is for the checksum to be used by the
	// caller, so check it here.
	c.Assert(resultMeta.FileMetadata.Checksum(), gc.NotNil)
	b, err := io.ReadAll(resultArchive)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(string(b), gc.Equals, "archive file testing")

	_, err = os.Stat(backupFilename)
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf("stat %s: no such file or directory", backupFilename))
}
