// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environment_test

import (
	gc "launchpad.net/gocheck"

	jujutesting "launchpad.net/juju-core/juju/testing"
	apitesting "launchpad.net/juju-core/state/api/testing"
)

type environmentSuite struct {
	jujutesting.JujuConnSuite
	*apitesting.EnvironWatcherTests
}

var _ = gc.Suite(&environmentSuite{})

func (s *environmentSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	stateAPI, _ := s.OpenAPIAsNewMachine(c)

	environmentAPI := stateAPI.Environment()
	c.Assert(environmentAPI, gc.NotNil)

	s.EnvironWatcherTests = apitesting.NewEnvironWatcherTests(
		environmentAPI, s.BackingState, apitesting.NoSecrets)
}
