// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package highavailability_test

import (
	stdtesting "testing"

	"github.com/juju/tc"
)

type clientSuite struct {
}

func TestClientSuite(t *stdtesting.T) {
	tc.Run(t, &clientSuite{})
}

func (s *clientSuite) TestStub(c *tc.C) {
	c.Skipf(`This suite is missing tests for the following scenarios:

 - Test s3 object store
 - Test enable HA error for no cloud local
 - Test enable HA error for no addresses
 - Test enable HA no error which verifies that virtual IPv4 addresses doesn't prevent enabling HA
 - Test enable HA no error which verifies that virtual IPv6 addresses doesn't prevent enabling HA
 - Test enable HA machine constraints
 - Test enable HA empty machine constraints
 - Test enable HA controller config constraints
 - Test enable HA controller config with file backed object store (currently not supported)
 - Test enable HA placement
 - Test enable HA placement --to
 - Test that killing a controller machine (machine-2) preserves the number of machines after enable-has is called again (machine-3 is added) to maintain 3 controller machines
 - Test that killing a controller machine (machine-4) preserves the number of machines after enable-has is called again (machine-5 is added) to maintain 5 controller machines
 - Test validate input options for enable-ha
 - Test enable HA with CAAS (currently not supported)
 - Test controller details
 `)
}
