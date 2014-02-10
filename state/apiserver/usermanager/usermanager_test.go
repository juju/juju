// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package usermanager_test

import (
	gc "launchpad.net/gocheck"
	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/apiserver/common"
	apiservertesting "launchpad.net/juju-core/state/apiserver/testing"
	"launchpad.net/juju-core/state/apiserver/usermanager"
)

type userManagerSuite struct {
	jujutesting.JujuConnSuite

	usermanager *usermanager.UserManagerAPI
	resources   *common.Resources
	authorizer  apiservertesting.FakeAuthorizer
}

var _ = gc.Suite(&userManagerSuite{})

func (s *userManagerSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.resources = common.NewResources()
	s.AddCleanup(func(_ *gc.C) { s.resources.StopAll() })

	s.authorizer = apiservertesting.FakeAuthorizer{
		Tag:      "user-admin",
		LoggedIn: true,
		Client:   true,
	}

	var err error
	s.usermanager, err = usermanager.NewUserManagerAPI(s.State, s.resources, s.authorizer)
	c.Assert(err, gc.IsNil)
}

func (s *userManagerSuite) TestNewUserManagerAcceptsClient(c *gc.C) {
	endPoint, err := usermanager.NewUserManagerAPI(s.State, s.resources, s.authorizer)
	c.Assert(err, gc.IsNil)
	c.Assert(endPoint, gc.NotNil)
}

func (s *userManagerSuite) TestNewUserManagerAPIRefusesNonClient(c *gc.C) {
	anAuthoriser := s.authorizer
	anAuthoriser.Client = false
	endPoint, err := usermanager.NewUserManagerAPI(s.State, s.resources, anAuthoriser)
	c.Assert(endPoint, gc.IsNil)
	c.Assert(err, gc.ErrorMatches, "permission denied")
}

func (s *userManagerSuite) TestAddUser(c *gc.C) {
	args := params.ModifyUsers{
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
	args := params.ModifyUsers{
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
}
