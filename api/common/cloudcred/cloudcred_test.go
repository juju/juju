// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package cloudcred

import (
	gc "gopkg.in/check.v1"
)

var _ = gc.Suite(&cloudcredsSuite{})

type cloudcredsSuite struct{}

func (s *cloudcredsSuite) TestIsVisibleAttribute(c *gc.C) {
	c.Assert(IsVisibleAttribute("ec2", "access-key", "access-key"), gc.Equals, true)
	c.Assert(IsVisibleAttribute("ec2", "access-key", "secret-key"), gc.Equals, false)
	c.Assert(IsVisibleAttribute("ec2", "unknown-auth", "access-key"), gc.Equals, false)
}
