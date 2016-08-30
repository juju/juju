// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package usermanager_test

import (
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/api/usermanager"
	"github.com/juju/juju/apiserver/params"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type usermanagerSuite struct {
	jujutesting.JujuConnSuite

	usermanager *usermanager.Client
}

var _ = gc.Suite(&usermanagerSuite{})

func (s *usermanagerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.usermanager = usermanager.NewClient(s.OpenControllerAPI(c))
	c.Assert(s.usermanager, gc.NotNil)
}

func (s *usermanagerSuite) TearDownTest(c *gc.C) {
	s.usermanager.Close()
	s.JujuConnSuite.TearDownTest(c)
}

func (s *usermanagerSuite) TestAddUser(c *gc.C) {
	tag, _, err := s.usermanager.AddUser("foobar", "Foo Bar", "password")
	c.Assert(err, jc.ErrorIsNil)

	user, err := s.State.User(tag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(user.Name(), gc.Equals, "foobar")
	c.Assert(user.DisplayName(), gc.Equals, "Foo Bar")
	c.Assert(user.PasswordValid("password"), jc.IsTrue)
}

func (s *usermanagerSuite) TestAddExistingUser(c *gc.C) {
	s.Factory.MakeUser(c, &factory.UserParams{Name: "foobar"})

	_, _, err := s.usermanager.AddUser("foobar", "Foo Bar", "password")
	c.Assert(err, gc.ErrorMatches, "failed to create user: user already exists")
}

func (s *usermanagerSuite) TestAddUserResponseError(c *gc.C) {
	usermanager.PatchResponses(s, s.usermanager,
		func(interface{}) error {
			return errors.New("call error")
		},
	)
	_, _, err := s.usermanager.AddUser("foobar", "Foo Bar", "password")
	c.Assert(err, gc.ErrorMatches, "call error")
}

func (s *usermanagerSuite) TestAddUserResultCount(c *gc.C) {
	usermanager.PatchResponses(s, s.usermanager,
		func(result interface{}) error {
			if result, ok := result.(*params.AddUserResults); ok {
				result.Results = make([]params.AddUserResult, 2)
				return nil
			}
			return errors.New("wrong result type")
		},
	)
	_, _, err := s.usermanager.AddUser("foobar", "Foo Bar", "password")
	c.Assert(err, gc.ErrorMatches, "expected 1 result, got 2")
}

func (s *usermanagerSuite) TestRemoveUser(c *gc.C) {
	tag, _, err := s.usermanager.AddUser("jjam", "Jimmy Jam", "password")
	c.Assert(err, jc.ErrorIsNil)

	// Ensure the user exists.
	user, err := s.State.User(tag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(user.Name(), gc.Equals, "jjam")
	c.Assert(user.DisplayName(), gc.Equals, "Jimmy Jam")

	// Delete the user.
	err = s.usermanager.RemoveUser(tag.Name())
	c.Assert(err, jc.ErrorIsNil)

	// Assert that the user is gone.
	_, err = s.State.User(tag)
	c.Assert(err, jc.Satisfies, errors.IsUserNotFound)

	err = user.Refresh()
	c.Check(err, jc.ErrorIsNil)
	c.Assert(user.IsDeleted(), jc.IsTrue)
}

func (s *usermanagerSuite) TestDisableUser(c *gc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{Name: "foobar"})

	err := s.usermanager.DisableUser(user.Name())
	c.Assert(err, jc.ErrorIsNil)

	err = user.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(user.IsDisabled(), jc.IsTrue)
}

func (s *usermanagerSuite) TestDisableUserBadName(c *gc.C) {
	err := s.usermanager.DisableUser("not!good")
	c.Assert(err, gc.ErrorMatches, `"not!good" is not a valid username`)
}

func (s *usermanagerSuite) TestEnableUser(c *gc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{Name: "foobar", Disabled: true})

	err := s.usermanager.EnableUser(user.Name())
	c.Assert(err, jc.ErrorIsNil)

	err = user.Refresh()
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(user.IsDisabled(), jc.IsFalse)
}

func (s *usermanagerSuite) TestEnableUserBadName(c *gc.C) {
	err := s.usermanager.EnableUser("not!good")
	c.Assert(err, gc.ErrorMatches, `"not!good" is not a valid username`)
}

func (s *usermanagerSuite) TestCantRemoveAdminUser(c *gc.C) {
	err := s.usermanager.DisableUser(s.AdminUserTag(c).Name())
	c.Assert(err, gc.ErrorMatches, "failed to disable user: cannot disable controller model owner")
}

func (s *usermanagerSuite) TestUserInfo(c *gc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{
		Name: "foobar", DisplayName: "Foo Bar"})

	obtained, err := s.usermanager.UserInfo([]string{"foobar"}, usermanager.AllUsers)
	c.Assert(err, jc.ErrorIsNil)
	expected := []params.UserInfo{
		{
			Username:    "foobar",
			DisplayName: "Foo Bar",
			Access:      "login",
			CreatedBy:   s.AdminUserTag(c).Name(),
			DateCreated: user.DateCreated(),
		},
	}

	c.Assert(obtained, jc.DeepEquals, expected)
}

func (s *usermanagerSuite) TestUserInfoMoreThanOneResult(c *gc.C) {
	usermanager.PatchResponses(s, s.usermanager,
		func(result interface{}) error {
			if result, ok := result.(*params.UserInfoResults); ok {
				result.Results = make([]params.UserInfoResult, 2)
				result.Results[0].Result = &params.UserInfo{Username: "first"}
				result.Results[1].Result = &params.UserInfo{Username: "second"}
				return nil
			}
			return errors.New("wrong result type")
		},
	)
	obtained, err := s.usermanager.UserInfo(nil, usermanager.AllUsers)
	c.Assert(err, jc.ErrorIsNil)

	expected := []params.UserInfo{
		{Username: "first"},
		{Username: "second"},
	}

	c.Assert(obtained, jc.DeepEquals, expected)
}

func (s *usermanagerSuite) TestUserInfoMoreThanOneError(c *gc.C) {
	usermanager.PatchResponses(s, s.usermanager,
		func(result interface{}) error {
			if result, ok := result.(*params.UserInfoResults); ok {
				result.Results = make([]params.UserInfoResult, 2)
				result.Results[0].Error = &params.Error{Message: "first error"}
				result.Results[1].Error = &params.Error{Message: "second error"}
				return nil
			}
			return errors.New("wrong result type")
		},
	)
	_, err := s.usermanager.UserInfo([]string{"foo", "bar"}, usermanager.AllUsers)
	c.Assert(err, gc.ErrorMatches, "foo: first error, bar: second error")
}

func (s *usermanagerSuite) TestSetUserPassword(c *gc.C) {
	tag := s.AdminUserTag(c)
	err := s.usermanager.SetPassword(tag.Name(), "new-password")
	c.Assert(err, jc.ErrorIsNil)
	user, err := s.State.User(tag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(user.PasswordValid("new-password"), jc.IsTrue)
}

func (s *usermanagerSuite) TestSetUserPasswordCanonical(c *gc.C) {
	tag := s.AdminUserTag(c)
	err := s.usermanager.SetPassword(tag.Canonical(), "new-password")
	c.Assert(err, jc.ErrorIsNil)
	user, err := s.State.User(tag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(user.PasswordValid("new-password"), jc.IsTrue)
}

func (s *usermanagerSuite) TestSetUserPasswordBadName(c *gc.C) {
	err := s.usermanager.SetPassword("not!good", "new-password")
	c.Assert(err, gc.ErrorMatches, `"not!good" is not a valid username`)
}
