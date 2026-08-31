// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

//go:build linux

package backups_test

import (
	"math"

	"github.com/juju/tc"

	"github.com/juju/juju/core/backups"
)

// Disk usage can only be determined on linux, so the free space check
// is only exercised here.

func (s *filesSuite) TestCheckSpaceForNotEnough(c *tc.C) {
	err := backups.CheckSpaceFor(c.MkDir(), math.MaxInt64/2)
	c.Assert(err, tc.ErrorMatches,
		`not enough free space in ".*"; want \d+MiB, have \d+MiB`)
}

func (s *filesSuite) TestCheckSpaceForEnough(c *tc.C) {
	err := backups.CheckSpaceFor(c.MkDir(), 1024)
	c.Assert(err, tc.ErrorIsNil)
}
