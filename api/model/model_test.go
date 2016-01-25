// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package model_test

import (
	gc "gopkg.in/check.v1"

	apitesting "github.com/juju/juju/api/testing"
	jujutesting "github.com/juju/juju/juju/testing"
)

type modelSuite struct {
	jujutesting.JujuConnSuite
	*apitesting.EnvironWatcherTests
}

var _ = gc.Suite(&modelSuite{})

func (s *modelSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	stateAPI, _ := s.OpenAPIAsNewMachine(c)

	modelAPI := stateAPI.Model()
	c.Assert(modelAPI, gc.NotNil)

	s.EnvironWatcherTests = apitesting.NewEnvironWatcherTests(
		modelAPI, s.BackingState, apitesting.NoSecrets)
}
