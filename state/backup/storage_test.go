// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backup_test

import (
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/state/backup"
)

func (b *BackupSuite) TestStorageName(c *gc.C) {
	c.Assert(backup.StorageName("foo"), gc.Equals, "/backups/foo")
	c.Assert(backup.StorageName("/foo/bar"), gc.Equals, "/backups/bar")
	c.Assert(backup.StorageName("foo/bar"), gc.Equals, "/backups/bar")
}
