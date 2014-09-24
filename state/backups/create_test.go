// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"os"

	gc "launchpad.net/gocheck"

	"github.com/juju/juju/state/backups"
	"github.com/juju/juju/state/backups/metadata"
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

func (s *createSuite) metadata(notes string) *metadata.Metadata {
	origin := metadata.NewOrigin("<env ID>", "<machine ID>", "<hostname>")
	return metadata.NewMetadata(*origin, notes, nil)
}

func (s *createSuite) TestLegacy(c *gc.C) {
	meta := s.metadata("")
	metadataFile, err := meta.AsJSONBuffer()
	c.Assert(err, gc.IsNil)
	_, testFiles, expected := s.createTestFiles(c)

	dumper := &TestDBDumper{}
	args := backups.NewTestCreateArgs(testFiles, dumper, metadataFile)
	result, err := backups.Create(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.NotNil)

	archiveFile, size, checksum := backups.ExposeCreateResult(result)
	c.Assert(archiveFile, gc.NotNil)

	// Check the result.
	file, ok := archiveFile.(*os.File)
	c.Assert(ok, gc.Equals, true)

	s.checkSize(c, file, size)
	s.checkChecksum(c, file, checksum)
	s.checkArchive(c, file, expected)
}

func (s *createSuite) TestMetadataFileMissing(c *gc.C) {
	var testFiles []string
	dumper := &TestDBDumper{}

	args := backups.NewTestCreateArgs(testFiles, dumper, nil)
	_, err := backups.Create(args)

	c.Check(err, gc.ErrorMatches, "missing metadataFile")
}
