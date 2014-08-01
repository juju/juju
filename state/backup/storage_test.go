// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backup_test

import (
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/state/backup"
	"github.com/juju/juju/testing"
)

var _ = gc.Suite(&storageSuite{})

type storageSuite struct {
	testing.BaseSuite
}

func (s *storageSuite) TestStorageName(c *gc.C) {
	c.Check(backup.StorageName("foo"), gc.Equals, "/backups/foo")
	c.Check(backup.StorageName("/foo/bar"), gc.Equals, "/backups/bar")
	c.Check(backup.StorageName("foo/bar"), gc.Equals, "/backups/bar")
}
