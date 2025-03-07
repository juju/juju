// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package caasapplication_test

import (
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"
)

var _ = gc.Suite(&CAASApplicationSuite{})

type CAASApplicationSuite struct {
	testing.IsolationSuite
}

func (s *CAASApplicationSuite) TestStub(c *gc.C) {
	c.Skip(`This suite is missing tests for the following scenarios:

 - Test adding a unit
 - Test adding an unassigned unit
 - Test unit reusing a name
 - Test doesn't reuse dead unit by name
 - Test find by provider ID
 - Test result contains agent config
 - Test dying application doesn't deploy unit
 - Test missing uuid arg
 - Test missing name arg
 - Test unit terminating agent will restart if desired replicas > 0
 - Test unit terminating agent will not restart if desired replicas == 0
`)
}
