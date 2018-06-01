// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"os"
	"path"
	"runtime"

	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state/backups"
	backupstesting "github.com/juju/juju/state/backups/testing"
)

type createSuite struct {
	LegacySuite
}

var _ = gc.Suite(&createSuite{}) // Register the suite.

type TestDBDumper struct {
	DumpDir string
}

func (d *TestDBDumper) Dump(dumpDir string) error {
	d.DumpDir = dumpDir
	return nil
}

func (s *createSuite) TestLegacy(c *gc.C) {
	if runtime.GOOS == "windows" {
		c.Skip("bug 1403084: Currently does not work on windows, see comments inside backups.create function")
	}
	meta := backupstesting.NewMetadataStarted()
	metadataFile, err := meta.AsJSONBuffer()
	c.Assert(err, jc.ErrorIsNil)
	backupDir := c.MkDir()
	_, testFiles, expected := s.createTestFiles(c)

	dumper := &TestDBDumper{}
	args := backups.NewTestCreateArgs(backupDir, testFiles, dumper, metadataFile, true)
	result, err := backups.Create(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.NotNil)

	archiveFile, size, checksum, _ := backups.ExposeCreateResult(result)
	c.Assert(archiveFile, gc.NotNil)

	// Check the result.
	file, ok := archiveFile.(*os.File)
	c.Assert(ok, jc.IsTrue)

	s.checkSize(c, file, size)
	s.checkChecksum(c, file, checksum)
	s.checkArchive(c, file, expected)
}

func (s *createSuite) TestMetadataFileMissing(c *gc.C) {
	var backupDir string
	var testFiles []string
	dumper := &TestDBDumper{}

	args := backups.NewTestCreateArgs(backupDir, testFiles, dumper, nil, true)
	_, err := backups.Create(args)

	c.Check(err, gc.ErrorMatches, "missing metadataReader")
}

func (s *createSuite) TestNoDownloadTrue(c *gc.C) {
	s.testCreateNoDownload(c, true)
}

func (s *createSuite) TestNoDownloadFalse(c *gc.C) {
	s.testCreateNoDownload(c, false)
}

func (s *createSuite) testCreateNoDownload(c *gc.C, noDownload bool) {
	meta := backupstesting.NewMetadataStarted()
	metadataFile, err := meta.AsJSONBuffer()
	c.Assert(err, jc.ErrorIsNil)
	backupDir := c.MkDir()
	_, testFiles, _ := s.createTestFiles(c)

	dumper := &TestDBDumper{}
	args := backups.NewTestCreateArgs(backupDir, testFiles, dumper, metadataFile, noDownload)
	result, err := backups.Create(args)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(result, gc.NotNil)
	_, _, _, resultFilename := backups.ExposeCreateResult(result)
	dir, filename := path.Split(resultFilename)
	c.Assert(filename, gc.Equals, backups.TempFilename)
	c.Assert(dir, jc.Contains, backupDir)
	_, err = os.Stat(resultFilename)
	if noDownload {
		c.Assert(err, jc.Satisfies, os.IsNotExist)
	} else {
		c.Assert(err, jc.ErrorIsNil)
	}
}
