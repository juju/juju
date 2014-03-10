// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package usermanager_test

import (
	gc "launchpad.net/gocheck"
	jujutesting "launchpad.net/juju-core/juju/testing"
	"launchpad.net/juju-core/state"
	"launchpad.net/juju-core/state/api/params"
	"launchpad.net/juju-core/state/api/usermanager"
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
	errResults, err := s.usermanager.AddUser("foobar", "password")
	c.Assert(err, gc.IsNil)
	expectedResult := params.ErrorResults{Results: []params.ErrorResult{params.ErrorResult{Error: nil}}}
	c.Assert(errResults, gc.DeepEquals, expectedResult)
	_, err = s.State.User("foobar")
	c.Assert(err, gc.IsNil)
}

func (s *usermanagerSuite) TestRemoveUser(c *gc.C) {
	errResults, err := s.usermanager.AddUser("foobar", "password")
	c.Assert(err, gc.IsNil)
	expectedResult := params.ErrorResults{Results: []params.ErrorResult{params.ErrorResult{Error: nil}}}
	c.Assert(errResults, gc.DeepEquals, expectedResult)
	_, err = s.State.User("foobar")
	c.Assert(err, gc.IsNil)

	errResults, err = s.usermanager.RemoveUser("foobar")
	c.Assert(err, gc.IsNil)
	c.Assert(errResults, gc.DeepEquals, expectedResult)
	user, err := s.State.User("foobar")
	c.Assert(user.IsDeactivated(), gc.Equals, true)
}

func (s *usermanagerSuite) TestAddExistingUser(c *gc.C) {
	_, err := s.usermanager.AddUser("foobar", "password")
	c.Assert(err, gc.IsNil)

	// Try adding again
	errResults, err := s.usermanager.AddUser("foobar", "password")
	expectedResult := params.ErrorResults{
		Results: []params.ErrorResult{
			params.ErrorResult{
				Error: &params.Error{
					Message: "Failed to create user: user already exists"}}}}
	c.Assert(errResults, gc.DeepEquals, expectedResult)
}

func (s *usermanagerSuite) TestCantRemoveAdminUser(c *gc.C) {
	errResults, err := s.usermanager.RemoveUser(state.AdminUser)
	c.Assert(err, gc.IsNil)
	expectedResult := params.ErrorResults{
		Results: []params.ErrorResult{
			params.ErrorResult{
				Error: &params.Error{
					Message: "Failed to remove user: Can't deactivate admin user"}}}}
	c.Assert(errResults, gc.DeepEquals, expectedResult)
}
