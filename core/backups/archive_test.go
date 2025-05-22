// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/core/backups"
	"github.com/juju/juju/internal/testing"
)

type archiveSuite struct {
	testing.BaseSuite
}

func TestArchiveSuite(t *stdtesting.T) {
	tc.Run(t, &archiveSuite{})
}

func (s *archiveSuite) TestNewCanonoicalArchivePaths(c *tc.C) {
	ap := backups.NewCanonicalArchivePaths()

	c.Check(ap.ContentDir, tc.Equals, "juju-backup")
	c.Check(ap.FilesBundle, tc.Equals, "juju-backup/root.tar")
	c.Check(ap.DBDumpDir, tc.Equals, "juju-backup/dump")
	c.Check(ap.MetadataFile, tc.Equals, "juju-backup/metadata.json")
}

func (s *archiveSuite) TestNewNonCanonicalArchivePaths(c *tc.C) {
	ap := backups.NewNonCanonicalArchivePaths("/tmp")

	c.Check(ap.ContentDir, tc.SamePath, "/tmp/juju-backup")
	c.Check(ap.FilesBundle, tc.SamePath, "/tmp/juju-backup/root.tar")
	c.Check(ap.DBDumpDir, tc.SamePath, "/tmp/juju-backup/dump")
	c.Check(ap.MetadataFile, tc.SamePath, "/tmp/juju-backup/metadata.json")
}
