// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	. "launchpad.net/gocheck"

	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/utils"
)

type UserSuite struct {
	ConnSuite
}

var _ = Suite(&UserSuite{})

func (s *UserSuite) TestAddUserInvalidNames(c *C) {
	for _, name := range []string{
		"foo-bar",
		"",
		"0foo",
	} {
		u, err := s.State.AddUser(name, "password")
		c.Assert(err, ErrorMatches, `invalid user name "`+name+`"`)
		c.Assert(u, IsNil)
	}
}

func (s *UserSuite) TestAddUser(c *C) {
	u, err := s.State.AddUser("a", "b")
	c.Check(u, NotNil)
	c.Assert(err, IsNil)

	c.Assert(u.Name(), Equals, "a")
	c.Assert(u.PasswordValid("b"), Equals, true)

	u1, err := s.State.User("a")
	c.Check(u1, NotNil)
	c.Assert(err, IsNil)

	c.Assert(u1.Name(), Equals, "a")
	c.Assert(u1.PasswordValid("b"), Equals, true)
}

func (s *UserSuite) TestSetPassword(c *C) {
	u, err := s.State.AddUser("someuser", "")
	c.Assert(err, IsNil)

	testSetPassword(c, func() (state.Authenticator, error) {
		return s.State.User(u.Name())
	})
}

func (s *UserSuite) TestSetPasswordHash(c *C) {
	u, err := s.State.AddUser("someuser", "")
	c.Assert(err, IsNil)

	err = u.SetPasswordHash(utils.PasswordHash("foo"))
	c.Assert(err, IsNil)

	c.Assert(u.PasswordValid("foo"), Equals, true)
	c.Assert(u.PasswordValid("bar"), Equals, false)
}

func (s *UserSuite) TestName(c *C) {
	u, err := s.State.AddUser("someuser", "")
	c.Assert(err, IsNil)

	c.Assert(u.Name(), Equals, "someuser")
	c.Assert(u.Tag(), Equals, "user-someuser")
}
