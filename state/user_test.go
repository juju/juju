// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	gc "launchpad.net/gocheck"

	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/utils"
)

type UserSuite struct {
	ConnSuite
}

var _ = gc.Suite(&UserSuite{})

func (s *UserSuite) TestAddUserInvalidNames(c *gc.C) {
	for _, name := range []string{
		"foo-bar",
		"",
		"0foo",
	} {
		u, err := s.State.AddUser(name, "password")
		c.Assert(err, gc.ErrorMatches, `invalid user name "`+name+`"`)
		c.Assert(u, gc.IsNil)
	}
}

func (s *UserSuite) TestAddUser(c *gc.C) {
	u, err := s.State.AddUser("a", "b")
	c.Check(u, gc.NotNil)
	c.Assert(err, gc.IsNil)

	c.Assert(u.Name(), gc.Equals, "a")
	c.Assert(u.PasswordValid("b"), gc.Equals, true)

	u1, err := s.State.User("a")
	c.Check(u1, gc.NotNil)
	c.Assert(err, gc.IsNil)

	c.Assert(u1.Name(), gc.Equals, "a")
	c.Assert(u1.PasswordValid("b"), gc.Equals, true)
}

func (s *UserSuite) TestSetPassword(c *gc.C) {
	u, err := s.State.AddUser("someuser", "")
	c.Assert(err, gc.IsNil)

	testSetPassword(c, func() (state.Authenticator, error) {
		return s.State.User(u.Name())
	})
}

func (s *UserSuite) TestSetPasswordTracksSalt(c *gc.C) {
}

func (s *UserSuite) TestSetPasswordHash(c *gc.C) {
	u, err := s.State.AddUser("someuser", "")
	c.Assert(err, gc.IsNil)

	err = u.SetPasswordHash(utils.CompatPasswordHash("foo"))
	c.Assert(err, gc.IsNil)

	c.Assert(u.PasswordValid("foo"), gc.Equals, true)
	c.Assert(u.PasswordValid("bar"), gc.Equals, false)

	// User passwords should *not* use the fast PasswordHash function
	hash, err := utils.AgentPasswordHash("foo-12345678901234567890")
	c.Assert(err, gc.IsNil)
	err = u.SetPasswordHash(hash)
	c.Assert(err, gc.IsNil)

	c.Assert(u.PasswordValid("foo"), gc.Equals, false)
}

func (s *UserSuite) TestName(c *gc.C) {
	u, err := s.State.AddUser("someuser", "")
	c.Assert(err, gc.IsNil)

	c.Assert(u.Name(), gc.Equals, "someuser")
	c.Assert(u.Tag(), gc.Equals, "user-someuser")
}
