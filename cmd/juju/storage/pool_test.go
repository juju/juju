// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/storage"
)

var expectedPoolCommmandNames = []string{
	"create",
	"help",
	"list",
}

type poolSuite struct {
	HelpStorageSuite
}

var _ = gc.Suite(&poolSuite{})

func (s *poolSuite) TestPoolHelp(c *gc.C) {
	s.command = storage.NewPoolSuperCommand()
	s.assertHelp(c, expectedPoolCommmandNames)
}
