// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package firewaller_test

import (
	"github.com/juju/tc"

	"github.com/juju/juju/internal/testhelpers"
)

type firewallerSuite struct {
	testhelpers.IsolationSuite
}

var _ = tc.Suite(&firewallerSuite{})

func (s *firewallerSuite) TestStub(c *tc.C) {
	c.Skip(`This suite is missing tests for the following scenarios:

 - Test that the firewaller API fails when the user is not a controller user.
 - Test it fails if the unit is not found when getting the unit life.
 - Test it fails if the machine is not found when getting the instance id.
 - Test it fails if the machine is not provisioned when getting the instance id.
 - Test watching for machine, application and unit entries.
 - Test that watching doesn't fail on the first failure, but continues to watch the rest.
 - Test that watching for units.
 - Test that watching for machines.
 - Test assigning a machine
 - Test that manually provisioned machines are detected.
 - Test that manually provisioned machines don't fail on the first error, but continue to process the rest.
 - Test get expose info returns the exposed endpoints, with spaces and cidrs.
 - Test get expose info returns no info when clear exposed is called.
 - Test get expose info doesn't fail on the first error, but continues to process the rest.
	`)
}
