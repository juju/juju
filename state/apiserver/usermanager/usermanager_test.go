// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package usermanager_test

import (
	gc "launchpad.net/gocheck"

	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/api/params"
	apiservertesting "github.com/juju/juju/state/apiserver/testing"
	"github.com/juju/juju/state/apiserver/usermanager"
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
		Tag:      "user-admin",
		LoggedIn: true,
		Client:   true,
		Entity:   user,
	}
	s.usermanager, err = usermanager.NewUserManagerAPI(s.State, s.authorizer)
	c.Assert(err, gc.IsNil)
}

func (s *userManagerSuite) TestNewUserManagerAPIRefusesNonClient(c *gc.C) {
	anAuthoriser := s.authorizer
	anAuthoriser.Client = false
	endPoint, err := usermanager.NewUserManagerAPI(s.State, anAuthoriser)
	c.Assert(endPoint, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *userManagerSuite) TestAddUser(c *gc.C) {
	args := params.ModifyUsers{
		Changes: []params.ModifyUser{{
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
	args := params.ModifyUsers{
		Changes: []params.ModifyUser{{
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
	args := params.ModifyUsers{
		Changes: []params.ModifyUser{{
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
