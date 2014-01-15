// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package names_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/names"
	jc "launchpad.net/juju-core/testing/checkers"
)

type userSuite struct{}

var _ = gc.Suite(&userSuite{})

func (s *userSuite) TestUserTag(c *gc.C) {
	c.Assert(names.UserTag("admin"), gc.Equals, "user-admin")
}

func (s *userSuite) TestIsUser(c *gc.C) {
	c.Assert(names.IsUser("admin"), jc.IsTrue)
	c.Assert(names.IsUser("foo42"), jc.IsTrue)
	c.Assert(names.IsUser("not/valid"), jc.IsFalse)
	c.Assert(names.IsUser(""), jc.IsFalse)
}
