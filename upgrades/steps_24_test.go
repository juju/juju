// Copyright 2017 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/upgrades"
)

var v24 = version.MustParse("2.4.0")

type steps24Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps24Suite{})

func (s *steps24Suite) TestMoveOldAuditLog(c *gc.C) {
	step := findStateStep(c, v24, "move or drop the old audit log collection")
	// Logic for step itself is tested in state package.
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}

func (s *steps24Suite) TestCopyMongoSpaceToHASpaceConfig(c *gc.C) {
	step := findStateStep(c, v24, "move controller info Mongo space to controller config HA space if valid")
	// Logic for step itself is tested in state package.
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}

func (s *steps24Suite) TestCreateMissingApplicationConfig(c *gc.C) {
	step := findStateStep(c, v24, "create empty application settings for all applications")
	// Logic for step itself is tested in state package.
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}

func (s *steps24Suite) TestRemoveVotingMachineIds(c *gc.C) {
	step := findStateStep(c, v24, "remove votingmachineids")
	// Logic for step itself is tested in state package.
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}

func (s *steps24Suite) TestCloudModelCounts(c *gc.C) {
	step := findStateStep(c, v24, "add cloud model counts")
	// Logic for step itself is tested in state package.
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}
func (s *steps24Suite) TestBootstrapRaft(c *gc.C) {
	step := findStateStep(c, v24, "bootstrap raft cluster")
	// Logic for step itself is tested in raft_test.go.
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.Controller})
}
