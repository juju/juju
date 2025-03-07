// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package deployer_test

import (
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"
)

type deployerSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&deployerSuite{})

func (s *deployerSuite) TestStub(c *gc.C) {
	c.Skip(`This suite is missing tests for the following scenarios:
	
 - Test deployer fails with non-machine agent user
 - Test watch units
 - Test set passwords
 - Test life
 - Test remove
 - Test connection info
 - Test set status`)
}
