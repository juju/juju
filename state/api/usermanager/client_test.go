// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package usermanager_test

import (
	gc "launchpad.net/gocheck"
	jujutesting "launchpad.net/juju-core/juju/testing"
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
	c.Assert(errResults, gc.DeepEquals, params.ErrorResult{})
	_, err = s.State.User("foobar")
	c.Assert(err, gc.IsNil)
}

func (s *usermanagerSuite) TestRemoveUser(c *gc.C) {
	errResults, err := s.usermanager.AddUser("foobar", "password")
	c.Assert(err, gc.IsNil)
	c.Assert(errResults, gc.DeepEquals, params.ErrorResult{})
	_, err = s.State.User("foobar")
	c.Assert(err, gc.IsNil)

	errResults, err = s.usermanager.RemoveUser("foobar")
	c.Assert(err, gc.IsNil)
	c.Assert(errResults, gc.DeepEquals, params.ErrorResult{})
	user, err := s.State.User("foobar")
	c.Assert(user.IsInactive(), gc.Equals, true)
}
