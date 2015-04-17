// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state/backups"
	"github.com/juju/juju/testing"
)

type archiveSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&archiveSuite{})

func (s *archiveSuite) TestNewCanonoicalArchivePaths(c *gc.C) {
	ap := backups.NewCanonicalArchivePaths()

	c.Check(ap.ContentDir, gc.Equals, "juju-backup")
	c.Check(ap.FilesBundle, gc.Equals, "juju-backup/root.tar")
	c.Check(ap.DBDumpDir, gc.Equals, "juju-backup/dump")
	c.Check(ap.MetadataFile, gc.Equals, "juju-backup/metadata.json")
}

func (s *archiveSuite) TestNewNonCanonicalArchivePaths(c *gc.C) {
	ap := backups.NewNonCanonicalArchivePaths("/tmp")

	c.Check(ap.ContentDir, jc.SamePath, "/tmp/juju-backup")
	c.Check(ap.FilesBundle, jc.SamePath, "/tmp/juju-backup/root.tar")
	c.Check(ap.DBDumpDir, jc.SamePath, "/tmp/juju-backup/dump")
	c.Check(ap.MetadataFile, jc.SamePath, "/tmp/juju-backup/metadata.json")
}
