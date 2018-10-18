// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	"os/exec"

	"github.com/dustin/go-humanize"
	pkgmgr "github.com/juju/packaging/manager"
	jc "github.com/juju/testing/checkers"
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
	s.PatchValue(&upgrades.MinDiskSpaceMib, uint64(humanize.PiByte/humanize.MiByte))
	err := upgrades.PreUpgradeSteps(nil, &mockAgentConfig{dataDir: "/"}, false, false)
	c.Assert(err, gc.ErrorMatches, `not enough free disk space on "/" for upgrade: .* available, require 1073741824MiB`)
}

func (s *preupgradechecksSuite) TestUpdateDistroInfo(c *gc.C) {
	s.PatchValue(&upgrades.MinDiskSpaceMib, uint64(0))
	expectedAptCommandArgs := [][]string{
		{"update"},
		{"install", "distro-info"},
	}

	commandChan := s.HookCommandOutput(&pkgmgr.CommandOutput, nil, nil)
	err := upgrades.PreUpgradeSteps(nil, &mockAgentConfig{dataDir: "/"}, true, false)
	c.Assert(err, jc.ErrorIsNil)

	var commands []*exec.Cmd
	for i := 0; i < cap(expectedAptCommandArgs)+1; i++ {
		select {
		case cmd := <-commandChan:
			commands = append(commands, cmd)
		default:
			break
		}
	}
	if len(commands) != len(expectedAptCommandArgs) {
		c.Fatalf("expected %d commands, got %d", len(expectedAptCommandArgs), len(commands))
	}

	assertAptCommand := func(cmd *exec.Cmd, tailArgs ...string) {
		args := cmd.Args
		c.Assert(len(args), jc.GreaterThan, len(tailArgs))
		c.Assert(args[0], gc.Equals, "apt-get")
		c.Assert(args[len(args)-len(tailArgs):], gc.DeepEquals, tailArgs)
	}
	assertAptCommand(commands[0], "update")
	assertAptCommand(commands[1], "install", "distro-info")
}
