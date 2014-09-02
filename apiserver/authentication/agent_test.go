// Copyright 2014 Canonical Ltd. All rights reserved.
// Licensed under the AGPLv3, see LICENCE file for details.

package authentication_test

import (
	"github.com/juju/utils"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
)

type agentAuthenticatorSuite struct {
	testing.JujuConnSuite
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

	s.user = s.Factory.MakeUser(c, &factory.UserParams{
		Name:        "bobbrown",
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
	wordpress := s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	c.Assert(err, gc.IsNil)
	unit, err := wordpress.AddUnit()
	c.Assert(err, gc.IsNil)
	s.unit = unit
	password, err = utils.RandomPassword()
	c.Assert(err, gc.IsNil)
	err = unit.SetPassword(password)
	c.Assert(err, gc.IsNil)
	s.unitPassword = password

	// add relation
	wordpressEP, err := wordpress.Endpoint("db")
	c.Assert(err, gc.IsNil)
	mysql := s.AddTestingService(c, "mysql", s.AddTestingCharm(c, "mysql"))
	mysqlEP, err := mysql.Endpoint("server")
	c.Assert(err, gc.IsNil)
	s.relation, err = s.State.AddRelation(wordpressEP, mysqlEP)
	c.Assert(err, gc.IsNil)
}

// testCase is used for structured table based tests
type testCase struct {
	entity       state.Entity
	credentials  string
	nonce        string
	about        string
	errorMessage string
}

func (s *agentAuthenticatorSuite) TestValidLogins(c *gc.C) {
	testCases := []testCase{{
		entity:      s.user,
		credentials: "password",
		about:       "user login",
	}, {
		entity:      s.machine,
		credentials: s.machinePassword,
		nonce:       s.machineNonce,
		about:       "machine login",
	}, {
		entity:      s.unit,
		credentials: s.unitPassword,
		about:       "unit login",
	}}

	for i, t := range testCases {
		c.Logf("test %d: %s", i, t.about)
		var authenticator authentication.AgentAuthenticator
		err := authenticator.Authenticate(t.entity, t.credentials, t.nonce)
		c.Check(err, gc.IsNil)
	}
}

func (s *agentAuthenticatorSuite) TestInvalidLogins(c *gc.C) {
	testCases := []testCase{{
		entity:       s.relation,
		credentials:  "dummy-secret",
		about:        "relation login",
		errorMessage: "invalid request",
	}, {
		entity:       s.user,
		credentials:  "wrongpassword",
		about:        "user login for nonexistant user",
		errorMessage: "invalid entity name or password",
	}, {
		entity:       s.machine,
		credentials:  s.machinePassword,
		nonce:        "123",
		about:        "machine login",
		errorMessage: "machine 0 is not provisioned",
	}, {
		entity:       s.user,
		credentials:  "wrong-secret",
		about:        "user login for nonexistant user",
		errorMessage: "invalid entity name or password",
	}}

	for i, t := range testCases {
		c.Logf("test %d: %s", i, t.about)
		var authenticator authentication.AgentAuthenticator
		err := authenticator.Authenticate(t.entity, t.credentials, t.nonce)
		c.Check(err, gc.ErrorMatches, t.errorMessage)
	}
}
