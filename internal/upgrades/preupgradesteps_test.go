// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	"github.com/dustin/go-humanize"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/internal/upgrades"
	"github.com/juju/juju/testing"
)

type preupgradechecksSuite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&preupgradechecksSuite{})

func (s *preupgradechecksSuite) TestCheckFreeDiskSpace(c *gc.C) {
	// Expect an impossibly large amount of free disk.
	s.PatchValue(&upgrades.MinDiskSpaceMib, uint64(humanize.PiByte/humanize.MiByte))
	err := upgrades.PreUpgradeSteps(&mockAgentConfig{dataDir: "/"}, false)
	c.Assert(err, gc.ErrorMatches, `not enough free disk space on "/" for upgrade: .* available, require 1073741824MiB`)
}
