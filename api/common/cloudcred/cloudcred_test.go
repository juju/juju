// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudcred

import (
	"testing"

	"github.com/juju/tc"
)

func TestCloudSpecSuite(t *testing.T) {
	tc.Run(t, &CloudCredsSuite{})
}

type CloudCredsSuite struct{}

func (s *CloudCredsSuite) TestIsVisibleAttribute(c *tc.C) {
	c.Assert(IsVisibleAttribute("ec2", "access-key", "access-key"), tc.Equals, true)
	c.Assert(IsVisibleAttribute("ec2", "access-key", "secret-key"), tc.Equals, false)
	c.Assert(IsVisibleAttribute("ec2", "unknown-auth", "access-key"), tc.Equals, false)
}
