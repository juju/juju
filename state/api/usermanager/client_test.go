// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package usermanager_test

import (
	gc "launchpad.net/gocheck"

	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/api/params"
	"github.com/juju/juju/state/api/usermanager"
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
	err := s.APIState.Call("UserManager", "", "AddUser", userArgs, results)
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
	c.Assert(err, gc.ErrorMatches, "Failed to remove user: Can't deactivate admin user")
}
