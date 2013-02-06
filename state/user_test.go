package state_test

import (
	. "launchpad.net/gocheck"
	"launchpad.net/juju-core/state"
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

	testSetPassword(c, func() (state.AuthEntity, error) {
		return s.State.User(u.Name())
	})
}
