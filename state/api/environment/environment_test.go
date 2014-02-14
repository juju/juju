// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package environment_test

import (
	gc "launchpad.net/gocheck"

	jujutesting "launchpad.net/juju-core/juju/testing"
	commontesting "launchpad.net/juju-core/state/api/common/testing"
)

type environmentSuite struct {
	jujutesting.JujuConnSuite
	*commontesting.EnvironWatcherTest
}

var _ = gc.Suite(&environmentSuite{})

func (s *environmentSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	stateAPI, _ := s.OpenAPIAsNewMachine(c)

	environmentAPI := stateAPI.Environment()
	c.Assert(environmentAPI, gc.NotNil)

	s.EnvironWatcherTest = commontesting.NewEnvironWatcherTest(
		environmentAPI, s.State, s.BackingState, commontesting.NoSecrets)
}
