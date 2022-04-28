// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/upgrades"
)

var v2929 = version.MustParse("2.9.29")

type steps2929Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps2929Suite{})

func (s *steps2929Suite) TestUpdateOperationWithEnqueuingErrors(c *gc.C) {
	step := findStateStep(c, v2929, "update operations with enqueuing errors")
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}
