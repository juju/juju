// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrades_test

import (
	"github.com/juju/utils/featureflag"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/juju/osenv"
	"github.com/juju/juju/testing"
	"github.com/juju/juju/version"
)

type steps123Suite struct {
	testing.BaseSuite
}

var _ = gc.Suite(&steps123Suite{})

func (s *steps123Suite) TestStateStepsFor123(c *gc.C) {
	s.PatchEnvironment(osenv.JujuFeatureFlagEnvKey, "storage")
	featureflag.SetFlagsFromEnvironment(osenv.JujuFeatureFlagEnvKey)
	expected := []string{
		"add default storage pools",
		"drop old mongo indexes",
		"migrate envuuid to env-uuid in envUsersC",
		"move blocks from environment to state",
		"insert userenvnameC doc for each environment",
	}
	assertStateSteps(c, version.MustParse("1.23.0"), expected)
}

func (s *steps123Suite) TestStepsFor123(c *gc.C) {
	expected := []string{
		"add environment UUID to agent config",
	}
	assertSteps(c, version.MustParse("1.23.0"), expected)
}
