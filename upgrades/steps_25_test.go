// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/upgrades"
)

var v25 = version.MustParse("2.5.0")

type steps25Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps25Suite{})

func (s *steps25Suite) TestMigrateMachineIdField(c *gc.C) {
	step := findStateStep(c, v25, `migrate storage records to use "hostid" field`)
	// Logic for step itself is tested in state package.
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}

func (s *steps25Suite) TestMigrateLegacyLeases(c *gc.C) {
	step := findStateStep(c, v25, `migrate legacy leases into raft`)
	// Logic for step itself is tested in state package.
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.Controller})
}

func (s *steps25Suite) TestMigrateAddModelPermissions(c *gc.C) {
	step := findStateStep(c, v25, `migrate add-model permissions`)
	// Logic for step itself is tested in state package.
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}
