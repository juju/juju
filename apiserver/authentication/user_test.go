// Copyright 2014 Canonical Ltd. All rights reserved.
// Licensed under the AGPLv3, see LICENCE file for details.

package authentication_test

import (
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"

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
	c.Assert(err, jc.ErrorIsNil)
	nonce, err := utils.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetProvisioned("foo", nonce, nil)
	c.Assert(err, jc.ErrorIsNil)
	password, err := utils.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetPassword(password)
	c.Assert(err, jc.ErrorIsNil)
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
	c.Assert(err, jc.ErrorIsNil)
	password, err := utils.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)
	err = unit.SetPassword(password)
	c.Assert(err, jc.ErrorIsNil)
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
	c.Assert(err, jc.ErrorIsNil)
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
	c.Assert(err, jc.ErrorIsNil)
	mysql := s.AddTestingService(c, "mysql", s.AddTestingCharm(c, "mysql"))
	mysqlEP, err := mysql.Endpoint("server")
	c.Assert(err, jc.ErrorIsNil)
	relation, err := s.State.AddRelation(wordpressEP, mysqlEP)
	c.Assert(err, jc.ErrorIsNil)

	// Attempt relation login
	authenticator := &authentication.UserAuthenticator{}
	err = authenticator.Authenticate(relation, "dummy-secret", "")
	c.Assert(err, gc.ErrorMatches, "invalid request")

}
