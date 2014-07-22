// Copyright 2014 Canonical Ltd. All rights reserved.
// Licensed under the AGPLv3, see LICENCE file for details.

package authentication_test

import (
	gc "launchpad.net/gocheck"

	"github.com/juju/utils"

	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/state/apiserver/authentication"
	"github.com/juju/juju/testing/factory"
)

type agentAuthenticatorSuite struct {
	jujutesting.JujuConnSuite
	machinePassword string
	machineNonce    string
	unitPassword    string
	machine         *state.Machine
	user            *state.User
	unit            *state.Unit
	relation        *state.Relation
}

var _ = gc.Suite(&agentAuthenticatorSuite{})

func (s *agentAuthenticatorSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)

	fact := factory.NewFactory(s.State, c)
	s.user = fact.MakeUser(factory.UserParams{
		Username:    "bobbrown",
		DisplayName: "Bob Brown",
		Password:    "password",
	})

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
	s.machine = machine
	s.machinePassword = password
	s.machineNonce = nonce

	// add a unit for testing unit agent authentication
	charm := s.AddTestingCharm(c, "dummy")
	service, err := s.State.AddService("wordpress", "user-admin", charm, nil)
	c.Assert(err, gc.IsNil)
	unit, err := service.AddUnit()
	c.Assert(err, gc.IsNil)
	s.unit = unit
	password, err = utils.RandomPassword()
	c.Assert(err, gc.IsNil)
	err = unit.SetPassword(password)
	c.Assert(err, gc.IsNil)
	s.unitPassword = password

	// add relation
	wordpress := s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	wordpressEP, err := wordpress.Endpoint("db")
	c.Assert(err, gc.IsNil)
	mysql := s.AddTestingService(c, "mysql", s.AddTestingCharm(c, "mysql"))
	mysqlEP, err := mysql.Endpoint("server")
	c.Assert(err, gc.IsNil)
	s.relation, err = s.State.AddRelation(wordpressEP, mysqlEP)
	c.Assert(err, gc.IsNil)
}

func (s *agentAuthenticatorSuite) TestValidLogins(c *gc.C) {
	testCases := []struct {
		entity      state.Entity
		credentials string
		nonce       string
		about       string
	}{{
		s.user, "password", "", "user login",
	}, {
		s.machine, s.machinePassword, s.machineNonce,
		"machine login",
	}, {
		s.unit, s.unitPassword, "",
		"unit login",
	}}

	authenticator := &authentication.AgentAuthenticator{}

	for i, t := range testCases {
		c.Logf("test %d: %s", i, t.about)
		err := authenticator.Authenticate(t.entity, t.credentials, t.nonce)
		c.Check(err, gc.IsNil)
	}
}

func (s *agentAuthenticatorSuite) TestInvalidLogins(c *gc.C) {
	testCases := []struct {
		entity      state.Entity
		credentials string
		nonce       string
		about       string
		Error       string
	}{{
		s.relation, "dummy-secret", "",
		"relation login", "invalid entity name or password",
	}, {
		s.user, "wrongpassword", "",
		"user login for nonexistant user", "invalid entity name or password",
	}, {
		s.machine, s.machinePassword, "123",
		"machine login", "machine 0 is not provisioned",
	}, {
		s.user, "wrong-secret", "",
		"user login for nonexistant user", "invalid entity name or password",
	}}

	authenticator := &authentication.AgentAuthenticator{}

	for i, t := range testCases {
		c.Logf("test %d: %s", i, t.about)
		err := authenticator.Authenticate(t.entity, t.credentials, t.nonce)
		c.Check(err, gc.ErrorMatches, t.Error)
	}
}
