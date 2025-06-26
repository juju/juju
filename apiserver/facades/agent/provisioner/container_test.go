// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner_test

import (
	"testing"

	"github.com/juju/tc"
)

type containerProvisionerSuite struct {
	provisionerSuite
}

func TestContainerProvisionerSuite(t *testing.T) {
	tc.Run(t, &containerProvisionerSuite{})
}

func (s *containerProvisionerSuite) TestStub(c *tc.C) {
	c.Skip(`This suite is missing tests for the following scenarios:

 - Test prepare container interface info
 - Test host changes for containers
 `)
}
