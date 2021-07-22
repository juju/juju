// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/upgrades"
)

var v296 = version.MustParse("2.9.6")

type steps296Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps296Suite{})

func (s *steps296Suite) TestControllerInClusterCredentials(c *gc.C) {
	step := findStateStep(c, v296, `prepare k8s controller for in cluster credentials`)
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}
