// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/upgrades"
)

var v263 = version.MustParse("2.6.3")

type steps263Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps263Suite{})

func (s *steps263Suite) TestResetKVMMachineModificationStatusIdle(c *gc.C) {
	step := findStep(c, v263, "reset kvm machine modification status to idle")
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.AllMachines})
}

func (s *steps263Suite) TestUpdateK8sModelNameIndex(c *gc.C) {
	step := findStateStep(c, v263, `update model name index of k8s models`)
	// Logic for step itself is tested in state package.
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}
