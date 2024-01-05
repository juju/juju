// Copyright 2014 Canonical Ltd. All rights reserved.
// Licensed under the AGPLv3, see LICENCE file for details.

package authentication_test

import (
	"context"

	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils/v3"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
)

type agentAuthenticatorSuite struct {
	testing.ApiServerSuite
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
	s.ApiServerSuite.SetUpTest(c)

	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	s.user = f.MakeUser(c, &factory.UserParams{
		Name:        "bobbrown",
		DisplayName: "Bob Brown",
		Password:    "password",
	})

	// add machine for testing machine agent authentication
	st := s.ControllerModel(c).State()
	machine, err := st.AddMachine(state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	nonce, err := utils.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetProvisioned("foo", "", nonce, nil)
	c.Assert(err, jc.ErrorIsNil)
	password, err := utils.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetPassword(password)
	c.Assert(err, jc.ErrorIsNil)
	s.machine = machine
	s.machinePassword = password
	s.machineNonce = nonce

	// add a unit for testing unit agent authentication
	wordpress := f.MakeApplication(c, &factory.ApplicationParams{
		Name:  "wordpress",
		Charm: f.MakeCharm(c, &factory.CharmParams{Name: "wordpress"}),
	})
	unit, err := wordpress.AddUnit(state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	s.unit = unit
	password, err = utils.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)
	err = unit.SetPassword(password)
	c.Assert(err, jc.ErrorIsNil)
	s.unitPassword = password

	// add relation
	wordpressEP, err := wordpress.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)
	mysql := f.MakeApplication(c, nil)
	mysqlEP, err := mysql.Endpoint("server")
	c.Assert(err, jc.ErrorIsNil)
	s.relation, err = s.ControllerModel(c).State().AddRelation(wordpressEP, mysqlEP)
	c.Assert(err, jc.ErrorIsNil)
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

	st := s.ControllerModel(c).State()
	for i, t := range testCases {
		c.Logf("test %d: %s", i, t.about)
		var authenticator authentication.AgentAuthenticator
		entity, err := authenticator.Authenticate(context.Background(), st, authentication.AuthParams{
			AuthTag:     t.entity.Tag(),
			Credentials: t.credentials,
			Nonce:       t.nonce,
		})
		c.Assert(err, jc.ErrorIsNil)
		c.Assert(entity.Tag(), gc.DeepEquals, t.entity.Tag())
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
		errorMessage: "machine 0 not provisioned",
	}, {
		entity:       s.user,
		credentials:  "wrong-secret",
		about:        "user login for nonexistant user",
		errorMessage: "invalid entity name or password",
	}}

	st := s.ControllerModel(c).State()
	for i, t := range testCases {
		c.Logf("test %d: %s", i, t.about)
		var authenticator authentication.AgentAuthenticator
		entity, err := authenticator.Authenticate(context.Background(), st, authentication.AuthParams{
			AuthTag:     t.entity.Tag(),
			Credentials: t.credentials,
			Nonce:       t.nonce,
		})
		c.Assert(err, gc.ErrorMatches, t.errorMessage)
		c.Assert(entity, gc.IsNil)
	}
}
