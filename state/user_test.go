// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"regexp"

	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/state"
	"github.com/juju/juju/utils"
)

type UserSuite struct {
	ConnSuite
}

var _ = gc.Suite(&UserSuite{})

func (s *UserSuite) TestAddUserInvalidNames(c *gc.C) {
	for _, name := range []string{
		"",
		"b^b",
	} {
		u, err := s.State.AddUser(name, "ignored", "ignored")
		c.Assert(err, gc.ErrorMatches, `invalid user name "`+regexp.QuoteMeta(name)+`"`)
		c.Assert(u, gc.IsNil)
	}
}

func (s *UserSuite) addUser(c *gc.C, name, displayName, password string) *state.User {
	user, err := s.State.AddUser(name, displayName, password)
	c.Assert(err, gc.IsNil)
	c.Assert(user, gc.NotNil)
	c.Assert(user.Name(), gc.Equals, name)
	c.Assert(user.DisplayName(), gc.Equals, displayName)
	c.Assert(user.PasswordValid(password), jc.IsTrue)
	return user
}

func (s *UserSuite) makeUser(c *gc.C) *state.User {
	return s.addUser(c, "someuser", "displayName", "a-password")
}

func (s *UserSuite) TestAddUser(c *gc.C) {
	name := "f00-Bar.ram77"
	displayName := "Display"
	password := "password"

	s.addUser(c, name, displayName, password)

	user, err := s.State.User(name)
	c.Assert(err, gc.IsNil)
	c.Check(user, gc.NotNil)
	c.Assert(user.Name(), gc.Equals, name)
	c.Assert(user.DisplayName(), gc.Equals, displayName)
	c.Assert(user.PasswordValid(password), jc.IsTrue)
}

func (s *UserSuite) TestCheckUserExists(c *gc.C) {
	user := s.makeUser(c)
	exists, err := state.CheckUserExists(s.State, user.Name())
	c.Assert(err, gc.IsNil)
	c.Assert(exists, jc.IsTrue)
	exists, err = state.CheckUserExists(s.State, "notAUser")
	c.Assert(err, gc.IsNil)
	c.Assert(exists, jc.IsFalse)
}

func (s *UserSuite) TestSetPassword(c *gc.C) {
	user := s.makeUser(c)
	testSetPassword(c, func() (state.Authenticator, error) {
		return s.State.User(user.Name())
	})
}

func (s *UserSuite) TestAddUserSetsSalt(c *gc.C) {
	u := s.makeUser(c)
	salt, hash := state.GetUserPasswordSaltAndHash(u)
	c.Check(hash, gc.Not(gc.Equals), "")
	c.Check(salt, gc.Not(gc.Equals), "")
	c.Check(utils.UserPasswordHash("a-password", salt), gc.Equals, hash)
	c.Check(u.PasswordValid("a-password"), jc.IsTrue)
}

func (s *UserSuite) TestSetPasswordChangesSalt(c *gc.C) {
	u := s.makeUser(c)
	origSalt, origHash := state.GetUserPasswordSaltAndHash(u)
	c.Check(origSalt, gc.Not(gc.Equals), "")
	// Even though the password is the same, we take this opportunity to
	// update the salt
	u.SetPassword("a-password")
	newSalt, newHash := state.GetUserPasswordSaltAndHash(u)
	c.Check(newSalt, gc.Not(gc.Equals), "")
	c.Check(newSalt, gc.Not(gc.Equals), origSalt)
	c.Check(newHash, gc.Not(gc.Equals), origHash)
	c.Check(u.PasswordValid("a-password"), jc.IsTrue)
}

func (s *UserSuite) TestSetPasswordHash(c *gc.C) {
	u := s.makeUser(c)

	err := u.SetPasswordHash(utils.UserPasswordHash("foo", utils.CompatSalt), utils.CompatSalt)
	c.Assert(err, gc.IsNil)

	c.Assert(u.PasswordValid("foo"), jc.IsTrue)
	c.Assert(u.PasswordValid("bar"), jc.IsFalse)

	// User passwords should *not* use the fast PasswordHash function
	hash := utils.AgentPasswordHash("foo-12345678901234567890")
	c.Assert(err, gc.IsNil)
	err = u.SetPasswordHash(hash, "")
	c.Assert(err, gc.IsNil)

	c.Assert(u.PasswordValid("foo-12345678901234567890"), jc.IsFalse)
}

func (s *UserSuite) TestSetPasswordHashWithSalt(c *gc.C) {
	u := s.makeUser(c)

	err := u.SetPasswordHash(utils.UserPasswordHash("foo", "salted"), "salted")
	c.Assert(err, gc.IsNil)

	c.Assert(u.PasswordValid("foo"), jc.IsTrue)
	salt, hash := state.GetUserPasswordSaltAndHash(u)
	c.Assert(salt, gc.Equals, "salted")
	c.Assert(hash, gc.Not(gc.Equals), utils.UserPasswordHash("foo", utils.CompatSalt))
}

func (s *UserSuite) TestPasswordValidUpdatesSalt(c *gc.C) {
	u := s.makeUser(c)

	compatHash := utils.UserPasswordHash("foo", utils.CompatSalt)
	err := u.SetPasswordHash(compatHash, "")
	c.Assert(err, gc.IsNil)
	beforeSalt, beforeHash := state.GetUserPasswordSaltAndHash(u)
	c.Assert(beforeSalt, gc.Equals, "")
	c.Assert(beforeHash, gc.Equals, compatHash)
	c.Assert(u.PasswordValid("bar"), jc.IsFalse)
	// A bad password doesn't trigger a rewrite
	afterBadSalt, afterBadHash := state.GetUserPasswordSaltAndHash(u)
	c.Assert(afterBadSalt, gc.Equals, "")
	c.Assert(afterBadHash, gc.Equals, compatHash)
	// When we get a valid check, we then add a salt and rewrite the hash
	c.Assert(u.PasswordValid("foo"), jc.IsTrue)
	afterSalt, afterHash := state.GetUserPasswordSaltAndHash(u)
	c.Assert(afterSalt, gc.Not(gc.Equals), "")
	c.Assert(afterHash, gc.Not(gc.Equals), compatHash)
	c.Assert(afterHash, gc.Equals, utils.UserPasswordHash("foo", afterSalt))
	// running PasswordValid again doesn't trigger another rewrite
	c.Assert(u.PasswordValid("foo"), jc.IsTrue)
	lastSalt, lastHash := state.GetUserPasswordSaltAndHash(u)
	c.Assert(lastSalt, gc.Equals, afterSalt)
	c.Assert(lastHash, gc.Equals, afterHash)
}

func (s *UserSuite) TestDeactivate(c *gc.C) {
	u := s.makeUser(c)
	c.Assert(u.IsDeactivated(), gc.Equals, false)

	err := u.Deactivate()
	c.Assert(err, gc.IsNil)
	c.Assert(u.IsDeactivated(), gc.Equals, true)
	c.Assert(u.PasswordValid(""), gc.Equals, false)

}

func (s *UserSuite) TestCantDeactivateAdminUser(c *gc.C) {
	u, err := s.State.User(state.AdminUser)
	c.Assert(err, gc.IsNil)
	err = u.Deactivate()
	c.Assert(err, gc.ErrorMatches, "Can't deactivate admin user")
}
