// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/testing"
	"github.com/juju/juju/version"
)

type steps126Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps126Suite{})

func (s *steps126Suite) TestStepsFor126(c *gc.C) {
	expected := []string{
		"installed boolean needs to be set in the uniter local state",
	}
	assertSteps(c, version.MustParse("1.26.0"), expected)
}
