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
	expected := []string{}
	assertSteps(c, version.MustParse("1.26.0"), expected)
}

func (s *steps126Suite) TestStateStepsFor126(c *gc.C) {
	expected := []string{
		"add the version field to all settings docs",
		"add status to filesystem",
		"upgrade environment config",
		"update machine preferred addresses",
		"add default endpoint bindings to services",
	}
	assertStateSteps(c, version.MustParse("1.26.0"), expected)
}
