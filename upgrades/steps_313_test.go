// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/upgrades"
)

var v313 = version.MustParse("3.1.3")

type steps313Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps313Suite{})

func (s *steps313Suite) TestEnsureInitalRefCountForExternalSecretBackends(c *gc.C) {
	step := findStateStep(c, v313, "ensure initial refCount for external secret backends")
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}
