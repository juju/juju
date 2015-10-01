// Copyright 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"regexp"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
)

type UserSuite struct {
	ConnSuite
}

var _ = gc.Suite(&UserSuite{})

func (s *UserSuite) TestAddInvalidNames(c *gc.C) {
	for _, name := range []string{
		"",
		"a",
		"b^b",
		"a.",
		"a-",
		"user@local",
		"@ubuntuone",
	} {
		c.Logf("check invalid name %q", name)
		user, err := s.State.AddUser(name, "ignored", "ignored", "ignored")
		c.Check(err, gc.ErrorMatches, `invalid user name "`+regexp.QuoteMeta(name)+`"`)
		c.Check(user, gc.IsNil)
	}
}

func (s *UserSuite) TestAddUser(c *gc.C) {
	name := "f00-Bar.ram77"
	displayName := "Display"
	password := "password"
	creator := "admin"

	now := time.Now().Round(time.Second).UTC()

	user, err := s.State.AddUser(name, displayName, password, creator)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(user, gc.NotNil)
	c.Assert(user.Name(), gc.Equals, name)
	c.Assert(user.DisplayName(), gc.Equals, displayName)
	c.Assert(user.PasswordValid(password), jc.IsTrue)
	c.Assert(user.CreatedBy(), gc.Equals, creator)
	c.Assert(user.DateCreated().After(now) ||
		user.DateCreated().Equal(now), jc.IsTrue)
	lastLogin, err := user.LastLogin()
	c.Assert(err, jc.Satisfies, state.IsNeverLoggedInError)
	c.Assert(lastLogin, gc.DeepEquals, time.Time{})

	user, err = s.State.User(user.UserTag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(user, gc.NotNil)
	c.Assert(user.Name(), gc.Equals, name)
	c.Assert(user.DisplayName(), gc.Equals, displayName)
	c.Assert(user.PasswordValid(password), jc.IsTrue)
	c.Assert(user.CreatedBy(), gc.Equals, creator)
	c.Assert(user.DateCreated().After(now) ||
		user.DateCreated().Equal(now), jc.IsTrue)
	lastLogin, err = user.LastLogin()
	c.Assert(err, jc.Satisfies, state.IsNeverLoggedInError)
	c.Assert(lastLogin, gc.DeepEquals, time.Time{})
}

func (s *UserSuite) TestCheckUserExists(c *gc.C) {
	user := s.Factory.MakeUser(c, nil)
	exists, err := state.CheckUserExists(s.State, user.Name())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(exists, jc.IsTrue)
	exists, err = state.CheckUserExists(s.State, "notAUser")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(exists, jc.IsFalse)
}

func (s *UserSuite) TestString(c *gc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{Name: "foo"})
	c.Assert(user.String(), gc.Equals, "foo@local")
}

func (s *UserSuite) TestUpdateLastLogin(c *gc.C) {
	now := time.Now().Round(time.Second).UTC()
	user := s.Factory.MakeUser(c, nil)
	err := user.UpdateLastLogin()
	c.Assert(err, jc.ErrorIsNil)
	lastLogin, err := user.LastLogin()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(lastLogin.After(now) ||
		lastLogin.Equal(now), jc.IsTrue)
}

func (s *UserSuite) TestSetPassword(c *gc.C) {
	user := s.Factory.MakeUser(c, nil)
	testSetPassword(c, func() (state.Authenticator, error) {
		return s.State.User(user.UserTag())
	})
}

func (s *UserSuite) TestAddUserSetsSalt(c *gc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{Password: "a-password"})
	salt, hash := state.GetUserPasswordSaltAndHash(user)
	c.Assert(hash, gc.Not(gc.Equals), "")
	c.Assert(salt, gc.Not(gc.Equals), "")
	c.Assert(utils.UserPasswordHash("a-password", salt), gc.Equals, hash)
	c.Assert(user.PasswordValid("a-password"), jc.IsTrue)
}

func (s *UserSuite) TestSetPasswordChangesSalt(c *gc.C) {
	user := s.Factory.MakeUser(c, nil)
	origSalt, origHash := state.GetUserPasswordSaltAndHash(user)
	c.Assert(origSalt, gc.Not(gc.Equals), "")
	user.SetPassword("a-password")
	newSalt, newHash := state.GetUserPasswordSaltAndHash(user)
	c.Assert(newSalt, gc.Not(gc.Equals), "")
	c.Assert(newSalt, gc.Not(gc.Equals), origSalt)
	c.Assert(newHash, gc.Not(gc.Equals), origHash)
	c.Assert(user.PasswordValid("a-password"), jc.IsTrue)
}

func (s *UserSuite) TestDisable(c *gc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{Password: "a-password"})
	c.Assert(user.IsDisabled(), jc.IsFalse)

	err := user.Disable()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(user.IsDisabled(), jc.IsTrue)
	c.Assert(user.PasswordValid("a-password"), jc.IsFalse)

	err = user.Enable()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(user.IsDisabled(), jc.IsFalse)
	c.Assert(user.PasswordValid("a-password"), jc.IsTrue)
}

func (s *UserSuite) TestSetPasswordHash(c *gc.C) {
	user := s.Factory.MakeUser(c, nil)

	err := user.SetPasswordHash(utils.UserPasswordHash("foo", utils.CompatSalt), utils.CompatSalt)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(user.PasswordValid("foo"), jc.IsTrue)
	c.Assert(user.PasswordValid("bar"), jc.IsFalse)

	// User passwords should *not* use the fast PasswordHash function
	hash := utils.AgentPasswordHash("foo-12345678901234567890")
	c.Assert(err, jc.ErrorIsNil)
	err = user.SetPasswordHash(hash, "")
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(user.PasswordValid("foo-12345678901234567890"), jc.IsFalse)
}

func (s *UserSuite) TestSetPasswordHashWithSalt(c *gc.C) {
	user := s.Factory.MakeUser(c, nil)

	err := user.SetPasswordHash(utils.UserPasswordHash("foo", "salted"), "salted")
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(user.PasswordValid("foo"), jc.IsTrue)
	salt, hash := state.GetUserPasswordSaltAndHash(user)
	c.Assert(salt, gc.Equals, "salted")
	c.Assert(hash, gc.Not(gc.Equals), utils.UserPasswordHash("foo", utils.CompatSalt))
}

func (s *UserSuite) TestPasswordValidUpdatesSalt(c *gc.C) {
	user := s.Factory.MakeUser(c, nil)

	compatHash := utils.UserPasswordHash("foo", utils.CompatSalt)
	err := user.SetPasswordHash(compatHash, "")
	c.Assert(err, jc.ErrorIsNil)
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

func (s *UserSuite) TestCantDisableAdmin(c *gc.C) {
	user, err := s.State.User(s.Owner)
	c.Assert(err, jc.ErrorIsNil)
	err = user.Disable()
	c.Assert(err, gc.ErrorMatches, "cannot disable state server environment owner")
}

func (s *UserSuite) TestCaseSensitiveUsersErrors(c *gc.C) {
	s.Factory.MakeUser(c, &factory.UserParams{Name: "Bob"})

	_, err := s.State.AddUser(
		"boB", "ignored", "ignored", "ignored")
	c.Assert(errors.IsAlreadyExists(err), jc.IsTrue)
	c.Assert(err, gc.ErrorMatches, "user already exists")
}

func (s *UserSuite) TestCaseInsensitiveLookup(c *gc.C) {
	expectedUser := s.Factory.MakeUser(c, &factory.UserParams{Name: "Bob"})

	assertCaseInsensitiveLookup := func(name string) {
		userTag := names.NewUserTag(name)
		obtainedUser, err := s.State.User(userTag)
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(obtainedUser, gc.DeepEquals, expectedUser)
	}

	assertCaseInsensitiveLookup("bob")
	assertCaseInsensitiveLookup("bOb")
	assertCaseInsensitiveLookup("boB")
	assertCaseInsensitiveLookup("BOB")
}

func (s *UserSuite) TestAllUsers(c *gc.C) {
	// Create in non-alphabetical order.
	s.Factory.MakeUser(c, &factory.UserParams{Name: "conrad"})
	s.Factory.MakeUser(c, &factory.UserParams{Name: "adam"})
	s.Factory.MakeUser(c, &factory.UserParams{Name: "debbie", Disabled: true})
	s.Factory.MakeUser(c, &factory.UserParams{Name: "barbara"})
	s.Factory.MakeUser(c, &factory.UserParams{Name: "fred", Disabled: true})
	s.Factory.MakeUser(c, &factory.UserParams{Name: "erica"})
	// There is the existing state server owner called "test-admin"

	includeDeactivated := false
	users, err := s.State.AllUsers(includeDeactivated)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(users, gc.HasLen, 5)
	c.Check(users[0].Name(), gc.Equals, "adam")
	c.Check(users[1].Name(), gc.Equals, "barbara")
	c.Check(users[2].Name(), gc.Equals, "conrad")
	c.Check(users[3].Name(), gc.Equals, "erica")
	c.Check(users[4].Name(), gc.Equals, "test-admin")

	includeDeactivated = true
	users, err = s.State.AllUsers(includeDeactivated)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(users, gc.HasLen, 7)
	c.Check(users[0].Name(), gc.Equals, "adam")
	c.Check(users[1].Name(), gc.Equals, "barbara")
	c.Check(users[2].Name(), gc.Equals, "conrad")
	c.Check(users[3].Name(), gc.Equals, "debbie")
	c.Check(users[4].Name(), gc.Equals, "erica")
	c.Check(users[5].Name(), gc.Equals, "fred")
	c.Check(users[6].Name(), gc.Equals, "test-admin")
}
