// Copyright 2013, 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package state_test

import (
	"fmt"
	"regexp"
	"strings"
	"time"

	"github.com/juju/errors"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing"
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

	now := testing.NonZeroTime().Round(time.Second).UTC()

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
	exists, err = state.CheckUserExists(s.State, strings.ToUpper(user.Name()))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(exists, jc.IsTrue)
	exists, err = state.CheckUserExists(s.State, "notAUser")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(exists, jc.IsFalse)
}

func (s *UserSuite) TestString(c *gc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{Name: "foo"})
	c.Assert(user.String(), gc.Equals, "foo")
}

func (s *UserSuite) TestUpdateLastLogin(c *gc.C) {
	now := testing.NonZeroTime().Round(time.Second).UTC()
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

func (s *UserSuite) TestRemoveUserNonExistent(c *gc.C) {
	err := s.State.RemoveUser(names.NewUserTag("harvey"))
	c.Assert(errors.IsNotFound(err), jc.IsTrue)
}

func isDeletedUserError(err error) bool {
	_, ok := errors.Cause(err).(state.DeletedUserError)
	return ok
}

func (s *UserSuite) TestAllUsersSkipsDeletedUsers(c *gc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{Name: "one"})
	_ = s.Factory.MakeUser(c, &factory.UserParams{Name: "two"})
	_ = s.Factory.MakeUser(c, &factory.UserParams{Name: "three"})

	all, err := s.State.AllUsers(true)
	c.Check(err, jc.ErrorIsNil)
	c.Check(len(all), jc.DeepEquals, 4)

	var got []string
	for _, u := range all {
		got = append(got, u.Name())
	}
	c.Check(got, jc.SameContents, []string{"test-admin", "one", "two", "three"})

	s.State.RemoveUser(user.UserTag())

	all, err = s.State.AllUsers(true)
	got = nil
	for _, u := range all {
		got = append(got, u.Name())
	}
	c.Check(err, jc.ErrorIsNil)
	c.Check(len(all), jc.DeepEquals, 3)
	c.Check(got, jc.SameContents, []string{"test-admin", "two", "three"})

}

func (s *UserSuite) TestRemoveUser(c *gc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{Password: "so sekrit"})

	// Assert user exists and can authenticate.
	c.Assert(user.PasswordValid("so sekrit"), jc.IsTrue)

	// Look for the user.
	u, err := s.State.User(user.UserTag())
	c.Check(err, jc.ErrorIsNil)
	c.Assert(u, jc.DeepEquals, user)

	// Remove the user.
	err = s.State.RemoveUser(user.UserTag())
	c.Check(err, jc.ErrorIsNil)

	// Check that we cannot update last login.
	err = u.UpdateLastLogin()
	c.Check(err, gc.NotNil)
	c.Check(isDeletedUserError(err), jc.IsTrue)
	c.Check(err.Error(), jc.DeepEquals,
		fmt.Sprintf("cannot update last login: user %q is permanently deleted", user.Name()))

	// Check that we cannot set a password.
	err = u.SetPassword("should fail too")
	c.Check(err, gc.NotNil)
	c.Check(isDeletedUserError(err), jc.IsTrue)
	c.Check(err.Error(), jc.DeepEquals,
		fmt.Sprintf("cannot set password: user %q is permanently deleted", user.Name()))

	// Check that we cannot set the password hash.
	err = u.SetPasswordHash("also", "fail")
	c.Check(err, gc.NotNil)
	c.Check(isDeletedUserError(err), jc.IsTrue)
	c.Check(err.Error(), jc.DeepEquals,
		fmt.Sprintf("cannot set password hash: user %q is permanently deleted", user.Name()))

	// Check that we cannot validate a password.
	c.Check(u.PasswordValid("should fail"), jc.IsFalse)

	// Check that we cannot enable the user.
	err = u.Enable()
	c.Check(err, gc.NotNil)
	c.Check(isDeletedUserError(err), jc.IsTrue)
	c.Check(err.Error(), jc.DeepEquals,
		fmt.Sprintf("cannot enable: user %q is permanently deleted", user.Name()))

	// Check that we cannot disable the user.
	err = u.Disable()
	c.Check(err, gc.NotNil)
	c.Check(isDeletedUserError(err), jc.IsTrue)
	c.Check(err.Error(), jc.DeepEquals,
		fmt.Sprintf("cannot disable: user %q is permanently deleted", user.Name()))

	// Check again to verify the user cannot be retrieved.
	u, err = s.State.User(user.UserTag())
	c.Check(err, gc.ErrorMatches, `user "username-\d+" is permanently deleted`)
}

func (s *UserSuite) TestRemoveUserUppercaseName(c *gc.C) {
	name := "NameWithUppercase"
	user := s.Factory.MakeUser(c, &factory.UserParams{
		Name:     name,
		Password: "wow very sea cret",
	})

	// Assert user exists and can authenticate.
	c.Assert(user.PasswordValid("wow very sea cret"), jc.IsTrue)

	// Look for the user.
	u, err := s.State.User(user.UserTag())
	c.Check(err, jc.ErrorIsNil)
	c.Assert(u, jc.DeepEquals, user)

	// Remove the user.
	err = s.State.RemoveUser(user.UserTag())
	c.Check(err, jc.ErrorIsNil)

	// Check to verify the user cannot be retrieved.
	_, err = s.State.User(user.UserTag())
	c.Check(err, gc.ErrorMatches, fmt.Sprintf(`user "%s" is permanently deleted`, name))
}

func (s *UserSuite) TestRemoveUserRemovesUserAccess(c *gc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{Password: "so sekrit"})

	// Assert user exists and can authenticate.
	c.Assert(user.PasswordValid("so sekrit"), jc.IsTrue)

	s.State.SetUserAccess(user.UserTag(), s.Model.ModelTag(), permission.AdminAccess)
	s.State.SetUserAccess(user.UserTag(), s.State.ControllerTag(), permission.SuperuserAccess)

	uam, err := s.State.UserAccess(user.UserTag(), s.Model.ModelTag())
	c.Check(err, jc.ErrorIsNil)
	c.Check(uam.Access, gc.Equals, permission.AdminAccess)

	uac, err := s.State.UserAccess(user.UserTag(), s.State.ControllerTag())
	c.Check(err, jc.ErrorIsNil)
	c.Check(uac.Access, gc.Equals, permission.SuperuserAccess)

	// Look for the user.
	u, err := s.State.User(user.UserTag())
	c.Check(err, jc.ErrorIsNil)
	c.Check(u, jc.DeepEquals, user)

	// Remove the user.
	err = s.State.RemoveUser(user.UserTag())
	c.Check(err, jc.ErrorIsNil)

	uam, err = s.State.UserAccess(user.UserTag(), s.Model.ModelTag())
	c.Check(err, gc.ErrorMatches, fmt.Sprintf("user %q is permanently deleted", user.UserTag().Name()))

	uac, err = s.State.UserAccess(user.UserTag(), s.State.ControllerTag())
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf("user %q is permanently deleted", user.UserTag().Name()))
}

func (s *UserSuite) TestDisableUser(c *gc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{Password: "a-password"})
	c.Assert(user.IsDisabled(), jc.IsFalse)
	c.Assert(s.activeUsers(c), jc.DeepEquals, []string{"test-admin", user.Name()})

	err := user.Disable()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(user.IsDisabled(), jc.IsTrue)
	c.Assert(user.PasswordValid("a-password"), jc.IsFalse)
	c.Assert(s.activeUsers(c), jc.DeepEquals, []string{"test-admin"})

	err = user.Enable()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(user.IsDisabled(), jc.IsFalse)
	c.Assert(user.PasswordValid("a-password"), jc.IsTrue)
	c.Assert(s.activeUsers(c), jc.DeepEquals, []string{"test-admin", user.Name()})
}

func (s *UserSuite) TestDisableUserUppercaseName(c *gc.C) {
	name := "NameWithUppercase"
	user := s.Factory.MakeUser(c, &factory.UserParams{Password: "a-password", Name: name})
	c.Assert(user.IsDisabled(), jc.IsFalse)
	c.Assert(s.activeUsers(c), jc.DeepEquals, []string{name, "test-admin"})

	err := user.Disable()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(user.IsDisabled(), jc.IsTrue)
	c.Assert(user.PasswordValid("a-password"), jc.IsFalse)
	c.Assert(s.activeUsers(c), jc.DeepEquals, []string{"test-admin"})

	err = user.Enable()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(user.IsDisabled(), jc.IsFalse)
	c.Assert(user.PasswordValid("a-password"), jc.IsTrue)
	c.Assert(s.activeUsers(c), jc.DeepEquals, []string{name, "test-admin"})
}

func (s *UserSuite) TestDisableUserDisablesUserAccess(c *gc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{Password: "so sekrit"})

	// Assert user exists and can authenticate.
	c.Assert(user.PasswordValid("so sekrit"), jc.IsTrue)

	s.State.SetUserAccess(user.UserTag(), s.Model.ModelTag(), permission.AdminAccess)
	s.State.SetUserAccess(user.UserTag(), s.State.ControllerTag(), permission.SuperuserAccess)

	uam, err := s.State.UserAccess(user.UserTag(), s.Model.ModelTag())
	c.Check(err, jc.ErrorIsNil)
	c.Check(uam.Access, gc.Equals, permission.AdminAccess)

	uac, err := s.State.UserAccess(user.UserTag(), s.State.ControllerTag())
	c.Check(err, jc.ErrorIsNil)
	c.Check(uac.Access, gc.Equals, permission.SuperuserAccess)

	// Look for the user.
	u, err := s.State.User(user.UserTag())
	c.Check(err, jc.ErrorIsNil)
	c.Check(u, jc.DeepEquals, user)

	// Disable the user.
	err = u.Disable()
	c.Check(err, jc.ErrorIsNil)

	uam, err = s.State.UserAccess(user.UserTag(), s.Model.ModelTag())
	c.Check(err, gc.ErrorMatches, fmt.Sprintf("user %q is disabled", user.UserTag().Name()))

	uac, err = s.State.UserAccess(user.UserTag(), s.State.ControllerTag())
	c.Assert(err, gc.ErrorMatches, fmt.Sprintf("user %q is disabled", user.UserTag().Name()))

	// Re-enable the user.
	err = u.Refresh()
	c.Check(err, jc.ErrorIsNil)
	err = u.Enable()
	c.Check(err, jc.ErrorIsNil)

	uam, err = s.State.UserAccess(user.UserTag(), s.Model.ModelTag())
	c.Check(err, jc.ErrorIsNil)
	c.Check(uam.Access, gc.Equals, permission.AdminAccess)

	uac, err = s.State.UserAccess(user.UserTag(), s.State.ControllerTag())
	c.Check(err, jc.ErrorIsNil)
	c.Check(uac.Access, gc.Equals, permission.SuperuserAccess)
}

func (s *UserSuite) activeUsers(c *gc.C) []string {
	users, err := s.State.AllUsers(false)
	c.Assert(err, jc.ErrorIsNil)
	names := make([]string, len(users))
	for i, u := range users {
		names[i] = u.Name()
	}
	return names
}

func (s *UserSuite) TestSetPasswordHash(c *gc.C) {
	user := s.Factory.MakeUser(c, nil)

	salt, err := utils.RandomSalt()
	c.Assert(err, jc.ErrorIsNil)
	err = user.SetPasswordHash(utils.UserPasswordHash("foo", salt), salt)
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

func (s *UserSuite) TestSetPasswordHashUppercaseName(c *gc.C) {
	name := "NameWithUppercase"
	user := s.Factory.MakeUser(c, &factory.UserParams{Name: name})

	salt, err := utils.RandomSalt()
	c.Assert(err, jc.ErrorIsNil)
	err = user.SetPasswordHash(utils.UserPasswordHash("foo", salt), salt)
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
	salt, _ := state.GetUserPasswordSaltAndHash(user)
	c.Assert(salt, gc.Equals, "salted")
}

func (s *UserSuite) TestCantDisableAdmin(c *gc.C) {
	user, err := s.State.User(s.Owner)
	c.Assert(err, jc.ErrorIsNil)
	err = user.Disable()
	c.Assert(err, gc.ErrorMatches, "cannot disable controller model owner")
}

func (s *UserSuite) TestCaseSensitiveUsersErrors(c *gc.C) {
	s.Factory.MakeUser(c, &factory.UserParams{Name: "Bob"})

	_, err := s.State.AddUser(
		"boB", "ignored", "ignored", "ignored")
	c.Assert(err, gc.ErrorMatches, "username unavailable")
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
	// There is the existing controller owner called "test-admin"

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

func (s *UserSuite) TestAddUserNoSecretKey(c *gc.C) {
	u, err := s.State.AddUser("bob", "display", "pass", "admin")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(u.SecretKey(), gc.IsNil)
}

func (s *UserSuite) TestAddUserSecretKey(c *gc.C) {
	u, err := s.State.AddUserWithSecretKey("bob", "display", "admin")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(u.SecretKey(), gc.HasLen, 32)
	c.Assert(u.PasswordValid(""), jc.IsFalse)
}

func (s *UserSuite) TestSetPasswordClearsSecretKey(c *gc.C) {
	u, err := s.State.AddUserWithSecretKey("bob", "display", "admin")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(u.SecretKey(), gc.HasLen, 32)
	err = u.SetPassword("anything")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(u.SecretKey(), gc.IsNil)
	err = u.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(u.SecretKey(), gc.IsNil)
}

func (s *UserSuite) TestResetPassword(c *gc.C) {
	u, err := s.State.AddUserWithSecretKey("bob", "display", "admin")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(u.SecretKey(), gc.HasLen, 32)
	oldKey := u.SecretKey()

	key, err := u.ResetPassword()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(key, gc.Not(gc.DeepEquals), oldKey)
	c.Assert(key, gc.NotNil)
	c.Assert(u.SecretKey(), gc.DeepEquals, key)
}

func (s *UserSuite) TestResetPasswordUppercaseName(c *gc.C) {
	u, err := s.State.AddUserWithSecretKey("BobHasAnUppercaseName", "display", "admin")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(u.SecretKey(), gc.HasLen, 32)
	oldKey := u.SecretKey()

	key, err := u.ResetPassword()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(key, gc.Not(gc.DeepEquals), oldKey)
	c.Assert(key, gc.NotNil)
	c.Assert(u.SecretKey(), gc.DeepEquals, key)
}

func (s *UserSuite) TestResetPasswordFailIfDeactivated(c *gc.C) {
	u, err := s.State.AddUser("bob", "display", "pass", "admin")
	c.Assert(err, jc.ErrorIsNil)

	err = u.Disable()
	c.Assert(err, jc.ErrorIsNil)

	_, err = u.ResetPassword()
	c.Assert(err, gc.ErrorMatches, `cannot reset password for user "bob": user deactivated`)
	c.Assert(u.SecretKey(), gc.IsNil)
}

func (s *UserSuite) TestResetPasswordFailIfDeleted(c *gc.C) {
	u, err := s.State.AddUser("bob", "display", "pass", "admin")
	c.Assert(err, jc.ErrorIsNil)

	err = s.State.RemoveUser(u.Tag().(names.UserTag))
	c.Assert(err, jc.ErrorIsNil)

	_, err = u.ResetPassword()
	c.Assert(err, gc.ErrorMatches, `cannot reset password for user "bob": user "bob" is permanently deleted`)
	c.Assert(u.SecretKey(), gc.IsNil)
}

func (s *UserSuite) TestResetPasswordIfPasswordSet(c *gc.C) {
	u, err := s.State.AddUser("bob", "display", "pass", "admin")
	c.Assert(err, jc.ErrorIsNil)

	err = u.SetPassword("anything")
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(u.PasswordValid("anything"), jc.IsTrue)
	c.Assert(u.SecretKey(), gc.IsNil)

	key, err := u.ResetPassword()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(u.SecretKey(), gc.DeepEquals, key)
	c.Assert(u.PasswordValid("anything"), jc.IsFalse)
}
