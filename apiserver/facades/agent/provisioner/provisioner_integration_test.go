// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner_test

import (
	stdtesting "testing"

	"github.com/juju/tc"

	"github.com/juju/juju/juju/testing"
)

type provisionerSuite struct {
	testing.ApiServerSuite
}

func TestProvisionerSuite(t *stdtesting.T) {
	tc.Run(t, &provisionerSuite{})
}

func (s *provisionerSuite) TestStub(c *tc.C) {
	c.Skip(`This suite is missing tests for the following scenarios:

 - Calling AvailabilityZones with machines that have populated, empty, and nil AZ
 - Calling KeepInstance with variaety of results - true, false, not found, unauthorised.
 - Test the distribution group is correctly distributed
 - Test the distribution group is correctly distributed are grouped by machine ids
 - Test provisioner fails with non machine agent and non controller user
 - Test provisioner fails to set short passwords (service covers this)
 - Test life as machine agent (setting and getting)
 - Test life as a controller machine (setting and getting)
 - Test removing a machine
 - Test setting a machine status
 - Test setting a machine instance status
 - Test setting a machine modification status
 - Test machines with transient errors
 - Test machines with transient errors permission
 - Test machines with ensure dead
 - Test watch all containers
 - Test status (all entities)
 - Test instance status (all entities)
 - Test setting constraints
 - Test setting instance info
 - Test setting instance id
 - Test mark machines for removal
 - Test set supported containers
 - Test set supported containers permission errors
 - Test distribution group controller auth
 - Test distribution group machine agent auth
 - Test distribution group by machine id controller auth
 - Test distribution group by machine id machine agent auth
 - Test watch model machines
 - Test watch machine error retry
	`)
}

type withoutControllerSuite struct {
	provisionerSuite
}

func TestWithoutControllerSuite(t *stdtesting.T) {
	tc.Run(t, &withoutControllerSuite{})
}
