// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/storage"
)

var expectedFilesystemCommmandNames = []string{
	"help",
	"list",
}

type filesystemSuite struct {
	HelpStorageSuite
}

var _ = gc.Suite(&filesystemSuite{})

func (s *filesystemSuite) TestFilesystemHelp(c *gc.C) {
	s.command = storage.NewFilesystemSuperCommand()
	s.assertHelp(c, expectedFilesystemCommmandNames)
}
