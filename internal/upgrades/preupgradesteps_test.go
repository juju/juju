// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	stdtesting "testing"

	"github.com/dustin/go-humanize"
	"github.com/juju/tc"

	"github.com/juju/juju/internal/testing"
	"github.com/juju/juju/internal/upgrades"
)

type preupgradechecksSuite struct {
	testing.BaseSuite
}

func TestPreupgradechecksSuite(t *stdtesting.T) { tc.Run(t, &preupgradechecksSuite{}) }
func (s *preupgradechecksSuite) TestCheckFreeDiskSpace(c *tc.C) {
	// Expect an impossibly large amount of free disk.
	s.PatchValue(&upgrades.MinDiskSpaceMib, uint64(humanize.PiByte/humanize.MiByte))
	err := upgrades.PreUpgradeSteps(&mockAgentConfig{dataDir: "/"}, false)
	c.Assert(err, tc.ErrorMatches, `not enough free disk space on "/" for upgrade: .* available, require 1073741824MiB`)
}
