// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backup_test

import (
	"fmt"
	"path"

	gc "launchpad.net/gocheck"

	"github.com/juju/juju/state/backup"
)

var testExpectedTarContents = []expectedTarContents{
	{"TarDirectoryEmpty", ""},
	{"TarDirectoryPopulated", ""},
	{"TarDirectoryPopulated/TarSubFile1", "TarSubFile1"},
	{"TarDirectoryPopulated/TarDirectoryPopulatedSubDirectory", ""},
	{"TarFile1", "TarFile1"},
	{"TarFile2", "TarFile2"},
}

func (b *BackupSuite) TestTarFilesUncompressed(c *gc.C) {
	b.createTestFiles(c)
	outputTar := path.Join(b.cwd, "output_tar_file.tar")
	trimPath := fmt.Sprintf("%s/", b.cwd)
	shaSum, err := backup.TarFiles(b.testFiles, outputTar, trimPath, false)
	c.Check(err, gc.IsNil)
	fileShaSum := shaSumFile(c, outputTar)
	c.Assert(shaSum, gc.Equals, fileShaSum)
	b.removeTestFiles(c)
	b.assertTarContents(c, testExpectedTarContents, outputTar, false)
}

func (b *BackupSuite) TestTarFilesCompressed(c *gc.C) {
	b.createTestFiles(c)
	outputTarGz := path.Join(b.cwd, "output_tar_file.tgz")
	trimPath := fmt.Sprintf("%s/", b.cwd)
	shaSum, err := backup.TarFiles(b.testFiles, outputTarGz, trimPath, true)
	c.Check(err, gc.IsNil)

	fileShaSum := shaSumFile(c, outputTarGz)
	c.Assert(shaSum, gc.Equals, fileShaSum)

	b.assertTarContents(c, testExpectedTarContents, outputTarGz, true)
}
