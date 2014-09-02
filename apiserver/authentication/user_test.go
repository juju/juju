// Copyright 2014 Canonical Ltd. All rights reserved.
// Licensed under the AGPLv3, see LICENCE file for details.

package authentication_test

import (
	"github.com/juju/utils"
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/apiserver/authentication"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
)

type userAuthenticatorSuite struct {
	jujutesting.JujuConnSuite
}

var _ = gc.Suite(&userAuthenticatorSuite{})

func (s *userAuthenticatorSuite) TestMachineLoginFails(c *gc.C) {
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
	machinePassword := password

	// attempt machine login
	authenticator := &authentication.UserAuthenticator{}
	err = authenticator.Authenticate(machine, machinePassword, nonce)
	c.Assert(err, gc.ErrorMatches, "invalid request")
}

func (s *userAuthenticatorSuite) TestUnitLoginFails(c *gc.C) {
	// add a unit for testing unit agent authentication
	wordpress := s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	unit, err := wordpress.AddUnit()
	c.Assert(err, gc.IsNil)
	password, err := utils.RandomPassword()
	c.Assert(err, gc.IsNil)
	err = unit.SetPassword(password)
	c.Assert(err, gc.IsNil)
	unitPassword := password

	// Attempt unit login
	authenticator := &authentication.UserAuthenticator{}
	err = authenticator.Authenticate(unit, unitPassword, "")
	c.Assert(err, gc.ErrorMatches, "invalid request")
}

func (s *userAuthenticatorSuite) TestValidUserLogin(c *gc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{
		Name:        "bobbrown",
		DisplayName: "Bob Brown",
		Password:    "password",
	})

	// User login
	authenticator := &authentication.UserAuthenticator{}
	err := authenticator.Authenticate(user, "password", "")
	c.Assert(err, gc.IsNil)
}

func (s *userAuthenticatorSuite) TestUserLoginWrongPassword(c *gc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{
		Name:        "bobbrown",
		DisplayName: "Bob Brown",
		Password:    "password",
	})

	// User login
	authenticator := &authentication.UserAuthenticator{}
	err := authenticator.Authenticate(user, "wrongpassword", "")
	c.Assert(err, gc.ErrorMatches, "invalid entity name or password")

}

func (s *userAuthenticatorSuite) TestInvalidRelationLogin(c *gc.C) {

	// add relation
	wordpress := s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	wordpressEP, err := wordpress.Endpoint("db")
	c.Assert(err, gc.IsNil)
	mysql := s.AddTestingService(c, "mysql", s.AddTestingCharm(c, "mysql"))
	mysqlEP, err := mysql.Endpoint("server")
	c.Assert(err, gc.IsNil)
	relation, err := s.State.AddRelation(wordpressEP, mysqlEP)
	c.Assert(err, gc.IsNil)

	// Attempt relation login
	authenticator := &authentication.UserAuthenticator{}
	err = authenticator.Authenticate(relation, "dummy-secret", "")
	c.Assert(err, gc.ErrorMatches, "invalid request")

}
