// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"path/filepath"

	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state/backups"
	"github.com/juju/juju/testing"
)

type archiveSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&archiveSuite{})

func (s *archiveSuite) TestNewArchivePaths(c *gc.C) {
	ar := backups.NewArchivePaths(string(filepath.Separator)+"tmp", true)

	c.Check(ar.RootDir, gc.Equals, "/tmp")
	c.Check(ar.ContentDir, gc.Equals, "/tmp/juju-backup")
	c.Check(ar.FilesBundle, gc.Equals, "/tmp/juju-backup/root.tar")
	c.Check(ar.DBDumpDir, gc.Equals, "/tmp/juju-backup/dump")
	c.Check(ar.MetadataFile, gc.Equals, "/tmp/juju-backup/metadata.json")
}

func (s *archiveSuite) TestNewUnpackedArchivePaths(c *gc.C) {
	ar := backups.NewUnpackedArchivePaths("/tmp")

	c.Check(ar.RootDir, gc.Equals, "/tmp")
	c.Check(ar.ContentDir, gc.Equals, "/tmp/juju-backup")
	c.Check(ar.FilesBundle, gc.Equals, "/tmp/juju-backup/root.tar")
	c.Check(ar.DBDumpDir, gc.Equals, "/tmp/juju-backup/dump")
	c.Check(ar.MetadataFile, gc.Equals, "/tmp/juju-backup/metadata.json")
}

func (s *archiveSuite) TestNewPackedArchivePaths(c *gc.C) {
	ar := backups.NewPackedArchivePaths()

	c.Check(ar.RootDir, gc.Equals, "")
	c.Check(ar.ContentDir, gc.Equals, "juju-backup")
	c.Check(ar.FilesBundle, gc.Equals, "juju-backup/root.tar")
	c.Check(ar.DBDumpDir, gc.Equals, "juju-backup/dump")
	c.Check(ar.MetadataFile, gc.Equals, "juju-backup/metadata.json")
}
