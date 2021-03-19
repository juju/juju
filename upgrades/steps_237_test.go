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

var v237 = version.MustParse("2.3.7")

type steps237Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps237Suite{})

func (s *steps236Suite) TestRemoveContainerImageStreamFromNonModelSettings(c *gc.C) {
	step := findStateStep(c, v237, "ensure container-image-stream isn't set in applications")
	// Logic for step itself is tested in state package.
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}
