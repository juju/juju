// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/version"
)

type steps125Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps125Suite{})

func (s *steps125Suite) TestStateStepsFor125(c *gc.C) {
	expected := []string{
		"set hosted environment count to number of hosted environments",
	}
	assertStateSteps(c, version.MustParse("1.25.0"), expected)
}
