// Copyright 2021 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/upgrades"
)

var v289 = version.MustParse("2.8.9")

type steps2810Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps2810Suite{})

func (s *steps2810Suite) TestTranslateK8sServiceTypes(c *gc.C) {
	step := findStateStep(c, v289, "translate k8s service types")
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}
