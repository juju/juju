// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"os"

	gc "launchpad.net/gocheck"

	"github.com/juju/juju/state/backups"
)

type createSuite struct {
	LegacySuite
}

var _ = gc.Suite(&createSuite{}) // Register the suite.

func (s *createSuite) TestCreateLegacy(c *gc.C) {
	_, testFiles, expected := s.createTestFiles(c)
	s.patchSources(c, testFiles)

	dumper := backups.NewTestDBDumper()
	args := backups.NewTestCreateArgs(testFiles, dumper)
	result, err := backups.Create(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.NotNil)

	archiveFile, size, checksum := backups.DumpCreateResult(result)
	c.Assert(archiveFile, gc.NotNil)

	c.Assert(s.ranCommand, gc.Equals, true)

	// Check the result.
	file, ok := archiveFile.(*os.File)
	c.Assert(ok, gc.Equals, true)

	s.checkSize(c, file, size, 0)
	s.checkChecksum(c, file, checksum, "")
	s.checkArchive(c, file, expected)
}
