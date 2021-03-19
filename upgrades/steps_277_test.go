// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/version/v2"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/upgrades"
)

var v277 = version.MustParse("2.7.7")

type steps277Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps277Suite{})

func (s *steps277Suite) TestReplaceSpaceNameWithIDEndpointBindings(c *gc.C) {
	step := findStateStep(c, v277, "replace space name in endpointBindingDoc bindings with an space ID")
	c.Assert(step.Targets(), jc.DeepEquals, []upgrades.Target{upgrades.DatabaseMaster})
}
