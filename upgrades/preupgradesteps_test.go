// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	"github.com/dustin/go-humanize"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/upgrades"
)

type preupgradechecksSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&preupgradechecksSuite{})

func (s *preupgradechecksSuite) TestCheckFreeDiskSpace(c *gc.C) {
	// Expect an impossibly large amount of free disk.
	s.PatchValue(upgrades.MinDiskSpaceGib, 1000*humanize.EiByte/humanize.GiByte)
	err := upgrades.PreUpgradeSteps(nil, &mockAgentConfig{dataDir: "/"}, false)
	c.Assert(err, gc.ErrorMatches, "not enough free disk space for upgrade .*")
}
