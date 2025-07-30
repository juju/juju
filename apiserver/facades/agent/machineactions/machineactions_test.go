// Copyright 2016 Canonical Ltd.
// Copyright 2016 Cloudbase Solutions
// Licensed under the AGPLv3, see LICENCE file for details.

package machineactions_test

import (
	"testing"

	"github.com/juju/tc"
)

type FacadeSuite struct {
}

func TestFacadeSuite(t *testing.T) {
	tc.Run(t, &FacadeSuite{})
}

func (*FacadeSuite) TestStub(c *tc.C) {
	c.Skipf(`This suite is missing tests for the following scenarios:
	
 - Test accepts machine agent
 - Test fails for other agents
 - Test returns the running actions for the machine agent`)
}
