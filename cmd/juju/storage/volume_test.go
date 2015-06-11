// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package storage_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/cmd/juju/storage"
)

var expectedVolumeCommmandNames = []string{
	"help",
	"list",
}

type volumeSuite struct {
	HelpStorageSuite
}

var _ = gc.Suite(&volumeSuite{})

func (s *volumeSuite) TestVolumeHelp(c *gc.C) {
	s.command = storage.NewVolumeSuperCommand()
	s.assertHelp(c, expectedVolumeCommmandNames)
}
