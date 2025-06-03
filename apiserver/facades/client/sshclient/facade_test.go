// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshclient_test

import (
	stdtesting "testing"

	"github.com/juju/tc"
)

type facadeSuite struct{}

func TestFacadeSuite(t *stdtesting.T) {
	tc.Run(t, &facadeSuite{})
}

func (s *facadeSuite) TestStub(c *tc.C) {
	c.Skip(`Tests for this facade should be implemented when the facade is cut over to services`)
}
