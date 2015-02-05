// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/version"
)

type steps123Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps123Suite{})

func (s *steps123Suite) TestStateStepsFor123(c *gc.C) {
	expected := []string{
		"drop old mongo indexes",
	}
	assertStateSteps(c, version.MustParse("1.23.0"), expected)
}

func (s *steps123Suite) TestStepsFor123(c *gc.C) {
	expected := []string{
		"add environment UUID to agent config",
	}
	assertSteps(c, version.MustParse("1.23.0"), expected)
}
