// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package usermanager_test

import (
	gc "launchpad.net/gocheck"

	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state/api/params"
	apiservertesting "launchpad.net/juju-core/state/apiserver/testing"
	"launchpad.net/juju-core/state/apiserver/usermanager"
)

type userManagerSuite struct {
	jujutesting.JujuConnSuite

	usermanager *usermanager.UserManagerAPI
	authorizer  apiservertesting.FakeAuthorizer
}

var _ = gc.Suite(&userManagerSuite{})

func (s *userManagerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag:      "user-admin",
		LoggedIn: true,
		Client:   true,
	}

	var err error
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
	args := params.ModifyUser{
		Tag:      "foobar",
		Password: "password",
	}

	result, err := s.usermanager.AddUser(args)
	// Check that the call is succesful
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{})
	// Check that the call results in a new user being created
	user, err := s.State.User("foobar")
	c.Assert(err, gc.IsNil)
	c.Assert(user, gc.NotNil)
}

func (s *userManagerSuite) TestRemoveUser(c *gc.C) {
	args := params.ModifyUser{
		Tag:      "foobar",
		Password: "password",
	}
	_, err := s.usermanager.AddUser(args)
	c.Assert(err, gc.IsNil)
	user, err := s.State.User("foobar")
	c.Assert(user.IsInactive(), gc.Equals, false) // The user should be active

	result, err := s.usermanager.RemoveUser(args)
	c.Assert(err, gc.IsNil)
	c.Assert(result, gc.DeepEquals, params.ErrorResults{})
	user, err = s.State.User("foobar")
	c.Assert(err, gc.IsNil)
	c.Assert(user.IsInactive(), gc.Equals, true) //Removal makes the user inactive
	c.Assert(user.PasswordValid(args.Password), gc.Equals, true)
}

/* Since removing a user just sets them inactive you cannot add a user
that has been previously been removed
*/
func (s *userManagerSuite) TestCannotAddRemoveAdd(c *gc.C) {
	args := params.ModifyUser{
		Tag:      "foobar",
		Password: "password",
	}
	_, err := s.usermanager.AddUser(args)
	c.Assert(err, gc.IsNil)

	_, err = s.usermanager.RemoveUser(args)
	c.Assert(err, gc.IsNil)
	_, err = s.State.User("foobar")
	_, err = s.usermanager.AddUser(args)
	c.Assert(err, gc.NotNil)
}
