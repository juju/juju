// Copyright 2013 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package usermanager_test

import (
	gc "launchpad.net/gocheck"
	jujutesting "launchpad.net/juju-core/juju/testing"
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
