// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"os"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state/backups"
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

func (s *createSuite) TestCreateLegacy(c *gc.C) {
	_, testFiles, expected := s.createTestFiles(c)

	dumper := &TestDBDumper{}
	args := backups.NewTestCreateArgs(testFiles, dumper)
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
