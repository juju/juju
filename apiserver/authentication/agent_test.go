// Copyright 2014 Canonical Ltd. All rights reserved.
// Licensed under the AGPLv3, see LICENCE file for details.

package authentication_test

import (
	"github.com/juju/testing"
	gc "gopkg.in/check.v1"
)

type agentAuthenticatorSuite struct {
	testing.IsolationSuite
}

var _ = gc.Suite(&agentAuthenticatorSuite{})

func (s *agentAuthenticatorSuite) TestStub(c *gc.C) {
	c.Skip(`This suite is missing tests for the following scenarios:
- Valid unit login.
- Valid machine login.
- Invalid agent login as user.
- Invalid login for machine not provisioned.
- Login for invalid relation entity.
`)
}
