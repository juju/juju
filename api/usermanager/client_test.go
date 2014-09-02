// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package usermanager_test

import (
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/api/usermanager"
	"github.com/juju/juju/apiserver/params"
	ums "github.com/juju/juju/apiserver/usermanager"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
)

type usermanagerSuite struct {
	jujutesting.JujuConnSuite

	usermanager *usermanager.Client
}

var _ = gc.Suite(&usermanagerSuite{})

func (s *usermanagerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.usermanager = usermanager.NewClient(s.APIState)
	c.Assert(s.usermanager, gc.NotNil)
}

func (s *usermanagerSuite) TestAddUser(c *gc.C) {
	err := s.usermanager.AddUser("foobar", "Foo Bar", "password")
	c.Assert(err, gc.IsNil)
	_, err = s.State.User("foobar")
	c.Assert(err, gc.IsNil)
}

func (s *usermanagerSuite) TestAddUserOldClient(c *gc.C) {
	userArgs := params.EntityPasswords{
		Changes: []params.EntityPassword{{Tag: "foobar", Password: "password"}},
	}
	results := new(params.ErrorResults)
	// Here we explicitly call into the UserManager object using the base
	// APIState so as to be able to call the AddUser method with a different
	// type of argument.
	err := s.APIState.APICall("UserManager", 0, "", "AddUser", userArgs, results)
	c.Assert(err, gc.IsNil)
	_, err = s.State.User("foobar")
	c.Assert(err, gc.IsNil)
}

func (s *usermanagerSuite) TestRemoveUser(c *gc.C) {
	err := s.usermanager.AddUser("foobar", "Foo Bar", "password")
	c.Assert(err, gc.IsNil)
	_, err = s.State.User("foobar")
	c.Assert(err, gc.IsNil)

	err = s.usermanager.RemoveUser("foobar")
	c.Assert(err, gc.IsNil)
	user, err := s.State.User("foobar")
	c.Assert(user.IsDeactivated(), gc.Equals, true)
}

func (s *usermanagerSuite) TestAddExistingUser(c *gc.C) {
	err := s.usermanager.AddUser("foobar", "Foo Bar", "password")
	c.Assert(err, gc.IsNil)

	// Try adding again
	err = s.usermanager.AddUser("foobar", "Foo Bar", "password")
	c.Assert(err, gc.ErrorMatches, "failed to create user: user already exists")
}

func (s *usermanagerSuite) TestCantRemoveAdminUser(c *gc.C) {
	err := s.usermanager.RemoveUser(state.AdminUser)
	c.Assert(err, gc.ErrorMatches, "Failed to remove user: cannot deactivate admin user")
}

func (s *usermanagerSuite) TestUserInfo(c *gc.C) {
	tag := names.NewUserTag("foobar")
	user := s.Factory.MakeUser(c, &factory.UserParams{Name: tag.Id(), DisplayName: "Foo Bar"})

	obtained, err := s.usermanager.UserInfo(tag.String())
	c.Assert(err, gc.IsNil)
	expected := ums.UserInfoResult{
		Result: &ums.UserInfo{
			Username:    "foobar",
			DisplayName: "Foo Bar",
			CreatedBy:   "admin",
			DateCreated: user.DateCreated(),
		},
	}

	c.Assert(obtained, jc.DeepEquals, expected)
}

func (s *usermanagerSuite) TestUserInfoNoResults(c *gc.C) {
	cleanup := usermanager.PatchResponses(s.usermanager,
		func(interface{}) error {
			// do nothing, we get an empty result with no error
			return nil
		},
	)
	defer cleanup()
	tag := names.NewUserTag("foobar")
	_, err := s.usermanager.UserInfo(tag.String())
	c.Assert(err, gc.ErrorMatches, "expected 1 result, got 0")
}

func (s *usermanagerSuite) TestUserInfoMoreThanOneResult(c *gc.C) {
	cleanup := usermanager.PatchResponses(s.usermanager,
		func(result interface{}) error {
			if result, ok := result.(*ums.UserInfoResults); ok {
				result.Results = make([]ums.UserInfoResult, 2)
			}
			return nil
		},
	)
	defer cleanup()
	tag := names.NewUserTag("foobar")
	_, err := s.usermanager.UserInfo(tag.String())
	c.Assert(err, gc.ErrorMatches, "expected 1 result, got 2")
}

func (s *usermanagerSuite) TestSetUserPassword(c *gc.C) {
	err := s.usermanager.SetPassword("admin", "new-password")
	c.Assert(err, gc.IsNil)
	user, err := s.State.User("admin")
	c.Assert(err, gc.IsNil)
	c.Assert(user.PasswordValid("new-password"), gc.Equals, true)
}
