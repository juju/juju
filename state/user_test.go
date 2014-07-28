// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"regexp"
	"time"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
)

type UserSuite struct {
	ConnSuite
}

var _ = gc.Suite(&UserSuite{})

func (s *UserSuite) SetUpTest(c *gc.C) {
	s.ConnSuite.SetUpTest(c)
}

func (s *UserSuite) TestAddUserInvalidNames(c *gc.C) {
	for _, name := range []string{
		"",
		"b^b",
	} {
		u, err := s.State.AddUser(name, "ignored", "ignored", "ignored")
		c.Assert(err, gc.ErrorMatches, `invalid user name "`+regexp.QuoteMeta(name)+`"`)
		c.Assert(u, gc.IsNil)
	}
}

func (s *UserSuite) TestAddUser(c *gc.C) {
	name := "f00-Bar.ram77"
	displayName := "Display"
	password := "password"
	creator := "admin"

	now := time.Now().Round(time.Second).UTC()

	user, err := s.State.AddUser(name, displayName, password, creator)
	c.Assert(err, gc.IsNil)
	c.Assert(user, gc.NotNil)
	c.Assert(user.Name(), gc.Equals, name)
	c.Assert(user.DisplayName(), gc.Equals, displayName)
	c.Assert(user.PasswordValid(password), jc.IsTrue)
	c.Assert(user.CreatedBy(), gc.Equals, creator)
	c.Assert(user.DateCreated().After(now) ||
		user.DateCreated().Equal(now), jc.IsTrue)
	c.Assert(user.LastConnection(), gc.IsNil)

	user, err = s.State.User(name)
	c.Assert(err, gc.IsNil)
	c.Check(user, gc.NotNil)
	c.Assert(user.Name(), gc.Equals, name)
	c.Assert(user.DisplayName(), gc.Equals, displayName)
	c.Assert(user.PasswordValid(password), jc.IsTrue)
	c.Assert(user.CreatedBy(), gc.Equals, creator)
	c.Assert(user.DateCreated().After(now) ||
		user.DateCreated().Equal(now), jc.IsTrue)
	c.Assert(user.LastConnection(), gc.IsNil)
}

func (s *UserSuite) TestCheckUserExists(c *gc.C) {
	user := s.factory.MakeUser()
	exists, err := state.CheckUserExists(s.State, user.Name())
	c.Assert(err, gc.IsNil)
	c.Assert(exists, jc.IsTrue)
	exists, err = state.CheckUserExists(s.State, "notAUser")
	c.Assert(err, gc.IsNil)
	c.Assert(exists, jc.IsFalse)
}

func (s *UserSuite) TestUpdateLastConnection(c *gc.C) {
	now := time.Now().Round(time.Second).UTC()
	user := s.factory.MakeUser()
	err := user.UpdateLastConnection()
	c.Assert(err, gc.IsNil)
	c.Assert(user.LastConnection().After(now) ||
		user.LastConnection().Equal(now), jc.IsTrue)
}

func (s *UserSuite) TestSetPassword(c *gc.C) {
	user := s.factory.MakeUser()
	testSetPassword(c, func() (state.Authenticator, error) {
		return s.State.User(user.Name())
	})
}

func (s *UserSuite) TestAddUserSetsSalt(c *gc.C) {
	user := s.factory.MakeUser(factory.UserParams{Password: "a-password"})
	salt, hash := state.GetUserPasswordSaltAndHash(user)
	c.Check(hash, gc.Not(gc.Equals), "")
	c.Check(salt, gc.Not(gc.Equals), "")
	c.Check(utils.UserPasswordHash("a-password", salt), gc.Equals, hash)
	c.Check(user.PasswordValid("a-password"), jc.IsTrue)
}

func (s *UserSuite) TestSetPasswordChangesSalt(c *gc.C) {
	user := s.factory.MakeUser()
	origSalt, origHash := state.GetUserPasswordSaltAndHash(user)
	c.Check(origSalt, gc.Not(gc.Equals), "")
	// Even though the password is the same, we take this opportunity to
	// update the salt
	user.SetPassword("a-password")
	newSalt, newHash := state.GetUserPasswordSaltAndHash(user)
	c.Check(newSalt, gc.Not(gc.Equals), "")
	c.Check(newSalt, gc.Not(gc.Equals), origSalt)
	c.Check(newHash, gc.Not(gc.Equals), origHash)
	c.Check(user.PasswordValid("a-password"), jc.IsTrue)
}

func (s *UserSuite) TestSetPasswordHash(c *gc.C) {
	user := s.factory.MakeUser()

	err := user.SetPasswordHash(utils.UserPasswordHash("foo", utils.CompatSalt), utils.CompatSalt)
	c.Assert(err, gc.IsNil)

	c.Assert(user.PasswordValid("foo"), jc.IsTrue)
	c.Assert(user.PasswordValid("bar"), jc.IsFalse)

	// User passwords should *not* use the fast PasswordHash function
	hash := utils.AgentPasswordHash("foo-12345678901234567890")
	c.Assert(err, gc.IsNil)
	err = user.SetPasswordHash(hash, "")
	c.Assert(err, gc.IsNil)

	c.Assert(user.PasswordValid("foo-12345678901234567890"), jc.IsFalse)
}

func (s *UserSuite) TestSetPasswordHashWithSalt(c *gc.C) {
	user := s.factory.MakeUser()

	err := user.SetPasswordHash(utils.UserPasswordHash("foo", "salted"), "salted")
	c.Assert(err, gc.IsNil)

	c.Assert(user.PasswordValid("foo"), jc.IsTrue)
	salt, hash := state.GetUserPasswordSaltAndHash(user)
	c.Assert(salt, gc.Equals, "salted")
	c.Assert(hash, gc.Not(gc.Equals), utils.UserPasswordHash("foo", utils.CompatSalt))
}

func (s *UserSuite) TestPasswordValidUpdatesSalt(c *gc.C) {
	user := s.factory.MakeUser()

	compatHash := utils.UserPasswordHash("foo", utils.CompatSalt)
	err := user.SetPasswordHash(compatHash, "")
	c.Assert(err, gc.IsNil)
	beforeSalt, beforeHash := state.GetUserPasswordSaltAndHash(user)
	c.Assert(beforeSalt, gc.Equals, "")
	c.Assert(beforeHash, gc.Equals, compatHash)
	c.Assert(user.PasswordValid("bar"), jc.IsFalse)
	// A bad password doesn't trigger a rewrite
	afterBadSalt, afterBadHash := state.GetUserPasswordSaltAndHash(user)
	c.Assert(afterBadSalt, gc.Equals, "")
	c.Assert(afterBadHash, gc.Equals, compatHash)
	// When we get a valid check, we then add a salt and rewrite the hash
	c.Assert(user.PasswordValid("foo"), jc.IsTrue)
	afterSalt, afterHash := state.GetUserPasswordSaltAndHash(user)
	c.Assert(afterSalt, gc.Not(gc.Equals), "")
	c.Assert(afterHash, gc.Not(gc.Equals), compatHash)
	c.Assert(afterHash, gc.Equals, utils.UserPasswordHash("foo", afterSalt))
	// running PasswordValid again doesn't trigger another rewrite
	c.Assert(user.PasswordValid("foo"), jc.IsTrue)
	lastSalt, lastHash := state.GetUserPasswordSaltAndHash(user)
	c.Assert(lastSalt, gc.Equals, afterSalt)
	c.Assert(lastHash, gc.Equals, afterHash)
}

func (s *UserSuite) TestDeactivate(c *gc.C) {
	user := s.factory.MakeUser()
	c.Assert(user.IsDeactivated(), gc.Equals, false)

	err := user.Deactivate()
	c.Assert(err, gc.IsNil)
	c.Assert(user.IsDeactivated(), gc.Equals, true)
	c.Assert(user.PasswordValid(""), gc.Equals, false)

}

func (s *UserSuite) TestCantDeactivateAdminUser(c *gc.C) {
	user, err := s.State.User(state.AdminUser)
	c.Assert(err, gc.IsNil)
	err = user.Deactivate()
	c.Assert(err, gc.ErrorMatches, "Can't deactivate admin user")
}
