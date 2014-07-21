// Copyright 2014 Canonical Ltd. All rights reserved.
// Licensed under the AGPLv3, see LICENCE file for details.

package identityprovider_test

import (
	gc "launchpad.net/gocheck"

	"github.com/juju/names"
	"github.com/juju/utils"

	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/apiserver/identityprovider"
)

type userProviderSuite struct {
	jujutesting.JujuConnSuite
	machineTag      names.Tag
	machinePassword string
	machineNonce    string
	unitPassword    string
}

var _ = gc.Suite(&userProviderSuite{})

func (s *userProviderSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	// add machine for testing machine agent authentication
	machine, err := s.State.AddMachine("quantal", state.JobHostUnits)
	c.Assert(err, gc.IsNil)
	nonce, err := utils.RandomPassword()
	c.Assert(err, gc.IsNil)
	err = machine.SetProvisioned("foo", nonce, nil)
	c.Assert(err, gc.IsNil)
	password, err := utils.RandomPassword()
	c.Assert(err, gc.IsNil)
	err = machine.SetPassword(password)
	c.Assert(err, gc.IsNil)
	s.machineTag = machine.Tag()
	s.machinePassword = password
	s.machineNonce = nonce

	// add a unit for testing unit agent authentication
	charm := s.AddTestingCharm(c, "dummy")
	service, err := s.State.AddService("wordpress", "user-admin", charm, nil)
	c.Assert(err, gc.IsNil)
	unit, err := service.AddUnit()
	c.Assert(err, gc.IsNil)
	password, err = utils.RandomPassword()
	c.Assert(err, gc.IsNil)
	err = unit.SetPassword(password)
	c.Assert(err, gc.IsNil)
	s.unitPassword = password
}

func (s *userProviderSuite) TestValidLogins(c *gc.C) {
	testCases := []struct {
		tag         names.Tag
		credentials string
		nonce       string
		about       string
	}{{
		names.NewUserTag("admin"), "dummy-secret", "",
		"user login",
	}}

	provider := &identityprovider.UserIdentityProvider{}

	for i, t := range testCases {
		c.Logf("test %d: %s", i, t.about)
		err := provider.Login(s.State, t.tag, t.credentials, t.nonce)
		c.Check(err, gc.IsNil)
	}
}

func (s *userProviderSuite) TestInvalidLogins(c *gc.C) {
	testCases := []struct {
		tag         names.Tag
		credentials string
		nonce       string
		about       string
		Error       string
	}{{
		names.NewRelationTag("wordpress:loadbalancer"), "dummy-secret", "",
		"relation login", "relation tag cannot be authenticated with a user identity provider",
	}, {
		names.NewUserTag("bob"), "dummy-secret", "",
		"user login for nonexistant user", "invalid entity name or password",
	}, {
		s.machineTag, s.machinePassword, "123",
		"machine login", "machine tag cannot be authenticated with a user identity provider",
	}, {
		names.NewUserTag("admin"), "wrong-secret", "",
		"user login for nonexistant user", "invalid entity name or password",
	}, {
		names.NewUnitTag("wordpress/0"), s.unitPassword, "",
		"unit login", "unit tag cannot be authenticated with a user identity provider",
	}}

	provider := &identityprovider.UserIdentityProvider{}

	for i, t := range testCases {
		c.Logf("test %d: %s", i, t.about)
		err := provider.Login(s.State, t.tag, t.credentials, t.nonce)
		c.Check(err, gc.ErrorMatches, t.Error)
	}
}
