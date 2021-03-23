// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/upgrades"
)

var v236 = version.MustParse("2.3.6")

type steps236Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps236Suite{})

func (s *steps236Suite) TestEnsureContainerImageStreamDefault(c *gc.C) {
	step := findStateStep(c, v236, "ensure container-image-stream config defaults to released")
	// Logic for step itself is tested in state package.
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}
