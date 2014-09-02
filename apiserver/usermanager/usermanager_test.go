// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package usermanager_test

import (
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/apiserver/params"
	apiservertesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/apiserver/usermanager"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
)

type userManagerSuite struct {
	jujutesting.JujuConnSuite

	usermanager *usermanager.UserManagerAPI
	authorizer  apiservertesting.FakeAuthorizer
	user        *state.User
}

var _ = gc.Suite(&userManagerSuite{})

func (s *userManagerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	user, err := s.State.User("admin")
	c.Assert(err, gc.IsNil)
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: user.Tag(),
	}
	s.usermanager, err = usermanager.NewUserManagerAPI(s.State, nil, s.authorizer)
	c.Assert(err, gc.IsNil)
}

func (s *userManagerSuite) TestNewUserManagerAPIRefusesNonClient(c *gc.C) {
	anAuthoriser := s.authorizer
	anAuthoriser.Tag = names.NewMachineTag("1")
	endPoint, err := usermanager.NewUserManagerAPI(s.State, nil, anAuthoriser)
	c.Assert(endPoint, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *userManagerSuite) TestAddUser(c *gc.C) {
	args := usermanager.ModifyUsers{
		Changes: []usermanager.ModifyUser{{
			Username:    "foobar",
			DisplayName: "Foo Bar",
			Password:    "password",
		}}}

	result, err := s.usermanager.AddUser(args)
	// Check that the call is succesful
	c.Assert(err, gc.IsNil)
	c.Assert(result.Results, gc.HasLen, 1)
	c.Assert(result.Results[0], gc.DeepEquals, params.ErrorResult{Error: nil})
	// Check that the call results in a new user being created
	user, err := s.State.User("foobar")
	c.Assert(err, gc.IsNil)
	c.Assert(user, gc.NotNil)
	c.Assert(user.Name(), gc.Equals, "foobar")
	c.Assert(user.DisplayName(), gc.Equals, "Foo Bar")
}

func (s *userManagerSuite) TestRemoveUser(c *gc.C) {
	args := usermanager.ModifyUsers{
		Changes: []usermanager.ModifyUser{{
			Username:    "foobar",
			DisplayName: "Foo Bar",
			Password:    "password",
		}}}
	removeArg := params.Entity{
		Tag: "foobar",
	}
	removeArgs := params.Entities{Entities: []params.Entity{removeArg}}
	_, err := s.usermanager.AddUser(args)
	c.Assert(err, gc.IsNil)
	user, err := s.State.User("foobar")
	c.Assert(user.IsDeactivated(), gc.Equals, false) // The user should be active

	result, err := s.usermanager.RemoveUser(removeArgs)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{Results: []params.ErrorResult{params.ErrorResult{Error: nil}}})
	user, err = s.State.User("foobar")
	c.Assert(err, gc.IsNil)
	// Removal makes the user in active
	c.Assert(user.IsDeactivated(), gc.Equals, true)
	c.Assert(user.PasswordValid(args.Changes[0].Password), gc.Equals, false)
}

// Since removing a user just deacitvates them you cannot add a user
// that has been previously been removed
// TODO(mattyw) 2014-03-07 bug #1288745
func (s *userManagerSuite) TestCannotAddRemoveAdd(c *gc.C) {
	removeArg := params.Entity{
		Tag: "foobar",
	}
	args := usermanager.ModifyUsers{
		Changes: []usermanager.ModifyUser{{
			Username:    "foobar",
			DisplayName: "Foo Bar",
			Password:    "password",
		}}}
	removeArgs := params.Entities{Entities: []params.Entity{removeArg}}
	_, err := s.usermanager.AddUser(args)
	c.Assert(err, gc.IsNil)

	_, err = s.usermanager.RemoveUser(removeArgs)
	c.Assert(err, gc.IsNil)
	_, err = s.State.User("addremove")
	result, err := s.usermanager.AddUser(args)
	expectedError := apiservertesting.ServerError("failed to create user: user already exists")
	c.Assert(result, gc.DeepEquals, params.ErrorResults{
		Results: []params.ErrorResult{
			params.ErrorResult{expectedError}}})
}

func (s *userManagerSuite) TestUserInfoUsersExist(c *gc.C) {
	foobar := "foobar"
	barfoo := "barfoo"
	fooTag := names.NewUserTag(foobar)
	barTag := names.NewUserTag(barfoo)
	userFoo := s.Factory.MakeUser(c, &factory.UserParams{Name: foobar, DisplayName: "Foo Bar"})
	userBar := s.Factory.MakeUser(c, &factory.UserParams{Name: barfoo, DisplayName: "Bar Foo"})

	args := params.Entities{
		Entities: []params.Entity{{Tag: fooTag.String()}, {Tag: barTag.String()}},
	}
	results, err := s.usermanager.UserInfo(args)
	c.Assert(err, gc.IsNil)
	expected := usermanager.UserInfoResults{
		Results: []usermanager.UserInfoResult{
			{
				Result: &usermanager.UserInfo{
					Username:       "foobar",
					DisplayName:    "Foo Bar",
					CreatedBy:      "admin",
					DateCreated:    userFoo.DateCreated(),
					LastConnection: userFoo.LastLogin(),
				},
			}, {
				Result: &usermanager.UserInfo{
					Username:       "barfoo",
					DisplayName:    "Bar Foo",
					CreatedBy:      "admin",
					DateCreated:    userBar.DateCreated(),
					LastConnection: userBar.LastLogin(),
				},
			}},
	}

	c.Assert(results, jc.DeepEquals, expected)
}

func (s *userManagerSuite) TestUserInfoUserExists(c *gc.C) {
	foobar := "foobar"
	fooTag := names.NewUserTag(foobar)
	user := s.Factory.MakeUser(c, &factory.UserParams{Name: foobar, DisplayName: "Foo Bar"})

	args := params.Entities{
		Entities: []params.Entity{{Tag: fooTag.String()}},
	}
	results, err := s.usermanager.UserInfo(args)
	c.Assert(err, gc.IsNil)
	expected := usermanager.UserInfoResults{
		Results: []usermanager.UserInfoResult{
			{
				Result: &usermanager.UserInfo{
					Username:       "foobar",
					DisplayName:    "Foo Bar",
					CreatedBy:      "admin",
					DateCreated:    user.DateCreated(),
					LastConnection: user.LastLogin(),
				},
			},
		},
	}

	c.Assert(results, gc.DeepEquals, expected)
}

func (s *userManagerSuite) TestUserInfoUserDoesNotExist(c *gc.C) {
	userTag := names.NewUserTag("foobar")
	args := params.Entities{
		Entities: []params.Entity{{Tag: userTag.String()}},
	}
	results, err := s.usermanager.UserInfo(args)
	c.Assert(err, gc.IsNil)
	expected := usermanager.UserInfoResults{
		Results: []usermanager.UserInfoResult{
			{
				Result: nil,
				Error: &params.Error{
					Message: "permission denied",
					Code:    params.CodeUnauthorized,
				},
			},
		},
	}
	c.Assert(results, gc.DeepEquals, expected)
}

func (s *userManagerSuite) TestUserInfoMachineTagFails(c *gc.C) {
	userTag := names.NewMachineTag("0")
	args := params.Entities{
		Entities: []params.Entity{{Tag: userTag.String()}},
	}
	results, err := s.usermanager.UserInfo(args)
	c.Assert(err, gc.IsNil)
	expected := usermanager.UserInfoResults{
		Results: []usermanager.UserInfoResult{
			{
				Result: nil,
				Error: &params.Error{
					Message: `"machine-0" is not a valid user tag`,
					Code:    "",
				},
			},
		},
	}
	c.Assert(results, gc.DeepEquals, expected)
}

func (s *userManagerSuite) TestUserInfoNotATagFails(c *gc.C) {
	args := params.Entities{
		Entities: []params.Entity{{Tag: "notatag"}},
	}
	results, err := s.usermanager.UserInfo(args)
	c.Assert(err, gc.IsNil)
	expected := usermanager.UserInfoResults{
		Results: []usermanager.UserInfoResult{
			{
				Result: nil,
				Error: &params.Error{
					Message: `"notatag" is not a valid tag`,
					Code:    "",
				},
			},
		},
	}
	c.Assert(results, gc.DeepEquals, expected)
}

func (s *userManagerSuite) TestAgentUnauthorized(c *gc.C) {

	machine1, err := s.State.AddMachine("quantal", state.JobManageEnviron)
	c.Assert(err, gc.IsNil)

	// Create a FakeAuthorizer so we can check permissions,
	// set up assuming machine 1 has logged in.
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: machine1.Tag(),
	}

	s.usermanager, err = usermanager.NewUserManagerAPI(s.State, nil, s.authorizer)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *userManagerSuite) TestSetPassword(c *gc.C) {
	args := usermanager.ModifyUsers{
		Changes: []usermanager.ModifyUser{{
			Username: "admin",
			Password: "new-password",
		}}}
	results, err := s.usermanager.SetPassword(args)
	c.Assert(err, gc.IsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	c.Assert(results.Results[0], gc.DeepEquals, params.ErrorResult{Error: nil})

	adminUser, err := s.State.User("admin")
	c.Assert(err, gc.IsNil)

	c.Assert(adminUser.PasswordValid("new-password"), gc.Equals, true)
}

func (s *userManagerSuite) TestCannotSetPasswordWhenNotAUser(c *gc.C) {
	machine1, err := s.State.AddMachine("quantal", state.JobManageEnviron)
	c.Assert(err, gc.IsNil)
	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag: machine1.Tag(),
	}
	_, err = usermanager.NewUserManagerAPI(s.State, nil, s.authorizer)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *userManagerSuite) TestSetMultiplePasswords(c *gc.C) {
	args := usermanager.ModifyUsers{
		Changes: []usermanager.ModifyUser{
			{
				Username: "admin",
				Password: "new-password1",
			},
			{
				Username: "admin",
				Password: "new-password2",
			}}}
	results, err := s.usermanager.SetPassword(args)
	c.Assert(err, gc.IsNil)
	c.Assert(results.Results, gc.HasLen, 2)
	c.Assert(results.Results[0], gc.DeepEquals, params.ErrorResult{Error: nil})
	c.Assert(results.Results[1], gc.DeepEquals, params.ErrorResult{Error: nil})

	adminUser, err := s.State.User("admin")
	c.Assert(err, gc.IsNil)

	c.Assert(adminUser.PasswordValid("new-password2"), gc.Equals, true)
}

// Because at present all user are admins problems could be caused by allowing
// users to change other users passwords. For the time being we only allow
// the password of the current user to be changed
func (s *userManagerSuite) TestSetPasswordOnDifferentUser(c *gc.C) {
	s.Factory.MakeUser(c, &factory.UserParams{Name: "foobar"})
	args := usermanager.ModifyUsers{
		Changes: []usermanager.ModifyUser{{
			Username:    "foobar",
			DisplayName: "Foo Bar",
			Password:    "password",
		}}}
	results, err := s.usermanager.SetPassword(args)
	c.Assert(err, gc.IsNil)
	c.Assert(results.Results, gc.HasLen, 1)
	expectedError := apiservertesting.ServerError("Can only change the password of the current user (admin)")
	c.Assert(results.Results[0], gc.DeepEquals, params.ErrorResult{Error: expectedError})
}
