// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backupstorage_test

import (
	"time"

	gc "launchpad.net/gocheck"

	"github.com/juju/juju/state/backups/metadata"
	"github.com/juju/juju/state/backupstorage"
)

type backupSuite struct {
	baseSuite
}

var _ = gc.Suite(&backupSuite{})

func (s *backupSuite) TestNewBackupID(c *gc.C) {
	origin := metadata.NewOrigin("spam", "0", "localhost")
	started := time.Date(2014, time.Month(9), 12, 13, 19, 27, 0, time.UTC)
	meta := metadata.NewMetadata(*origin, "", &started)

	id := backupstorage.NewID(meta)

	c.Check(id, gc.Equals, "20140912-131927.spam")
}
