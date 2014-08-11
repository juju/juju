// Copyright 2013,2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package backups_test

import (
	"github.com/juju/utils/filestorage"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/state/backups"
	"github.com/juju/juju/testing"
)

type backupsSuite struct {
	testing.BaseSuite

	storage filestorage.FileStorage
	api     backups.Backups
}

var _ = gc.Suite(&backupsSuite{}) // Register the suite.

func (s *backupsSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)

	storage, err := filestorage.NewSimpleStorage(c.MkDir())
	c.Assert(err, gc.IsNil)
	s.storage = storage

	s.api = backups.NewBackups(s.storage)
}

func (s *backupsSuite) TestNewBackups(c *gc.C) {
	api := backups.NewBackups(s.storage)

	c.Check(api, gc.NotNil)
}
