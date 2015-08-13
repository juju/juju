// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/storage"
)

var expectedSubCommmandNames = []string{
	"add",
	"help",
	"list",
	"pool",
	"show",
	"volume",
}

type storageSuite struct {
	HelpStorageSuite
}

var _ = gc.Suite(&storageSuite{})

func (s *storageSuite) TestHelp(c *gc.C) {
	s.command = storage.NewSuperCommand()
	s.assertHelp(c, expectedSubCommmandNames)
}
