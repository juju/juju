// Copyright 2014 Canonical Ltd. All rights reserved.
// Licensed under the AGPLv3, see LICENCE file for details.

package authentication_test

import (
	"net/http"

	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/macaroon-bakery.v1/bakery"
	"gopkg.in/macaroon-bakery.v1/bakery/checkers"
	"gopkg.in/macaroon-bakery.v1/bakerytest"
	"gopkg.in/macaroon-bakery.v1/httpbakery"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/params"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
)

type userAuthenticatorSuite struct {
	jujutesting.JujuConnSuite
}

type entityFinder struct {
	entity state.Entity
}

func (f entityFinder) FindEntity(tag names.Tag) (state.Entity, error) {
	return f.entity, nil
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
	_, err = authenticator.Authenticate(nil, machine.Tag(), params.LoginRequest{
		Credentials: machinePassword,
		Nonce:       nonce,
	})
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
	_, err = authenticator.Authenticate(nil, unit.Tag(), params.LoginRequest{
		Credentials: unitPassword,
		Nonce:       "",
	})
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
	_, err := authenticator.Authenticate(s.State, user.Tag(), params.LoginRequest{
		Credentials: "password",
		Nonce:       "",
	})
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
	_, err := authenticator.Authenticate(s.State, user.Tag(), params.LoginRequest{
		Credentials: "wrongpassword",
		Nonce:       "",
	})
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
	_, err = authenticator.Authenticate(nil, relation.Tag(), params.LoginRequest{
		Credentials: "dummy-secret",
		Nonce:       "",
	})
	c.Assert(err, gc.ErrorMatches, "invalid request")

}

type macaroonAuthenticatorSuite struct {
	jujutesting.JujuConnSuite
	discharger *bakerytest.Discharger
	username   string
}

var _ = gc.Suite(&macaroonAuthenticatorSuite{})

func (s *macaroonAuthenticatorSuite) SetUpTest(c *gc.C) {
	s.JujuConnSuite.SetUpTest(c)
	s.discharger = bakerytest.NewDischarger(nil, s.Checker)
}

func (s *macaroonAuthenticatorSuite) TearDownTest(c *gc.C) {
	s.discharger.Close()
	s.JujuConnSuite.TearDownTest(c)
}

func (s *macaroonAuthenticatorSuite) Checker(req *http.Request, cond, arg string) ([]checkers.Caveat, error) {
	return []checkers.Caveat{checkers.DeclaredCaveat("username", s.username)}, nil
}

func (s *macaroonAuthenticatorSuite) TestReturnDischargeRequiredErrorIfNoMacaroons(c *gc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{
		Name:        "bobbrown",
		DisplayName: "Bob Brown",
		Password:    "password",
	})

	svc, err := bakery.NewService(bakery.NewServiceParams{
		Locator: s.discharger,
	})
	c.Assert(err, jc.ErrorIsNil)
	mac, err := svc.NewMacaroon("", nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	authenticator := &authentication.MacaroonAuthenticator{
		Service:          svc,
		IdentityLocation: s.discharger.Location(),
		Macaroon:         mac,
	}
	_, err = authenticator.Authenticate(nil, user.Tag(), params.LoginRequest{})
	c.Assert(err, gc.ErrorMatches, "discharge required")
	dischargeErr, ok := err.(*authentication.DischargeRequiredError)
	if !ok {
		c.Fatalf("DischargeRequiredError expected")
	}
	c.Assert(dischargeErr.Macaroon, gc.Not(gc.IsNil))
}

func (s *macaroonAuthenticatorSuite) TestAuthenticateSuccess(c *gc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{
		Name:        "bobbrown",
		DisplayName: "Bob Brown",
		Password:    "password",
	})

	s.username = "bobbrown"

	svc, err := bakery.NewService(bakery.NewServiceParams{
		Locator: s.discharger,
	})
	c.Assert(err, jc.ErrorIsNil)
	mac, err := svc.NewMacaroon("", nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	authenticator := &authentication.MacaroonAuthenticator{
		Service:          svc,
		IdentityLocation: s.discharger.Location(),
		Macaroon:         mac,
	}
	_, err = authenticator.Authenticate(nil, user.Tag(), params.LoginRequest{
		Credentials: "",
		Nonce:       "",
		Macaroons:   nil,
	})
	dischargeErr := err.(*authentication.DischargeRequiredError)
	client := httpbakery.NewClient()
	ms, err := client.DischargeAll(dischargeErr.Macaroon)
	c.Assert(err, jc.ErrorIsNil)
	entity, err := authenticator.Authenticate(s.State, user.Tag(), params.LoginRequest{
		Credentials: "",
		Nonce:       "",
		Macaroons:   ms,
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(entity.Tag(), gc.Equals, user.Tag())
}

func (s *macaroonAuthenticatorSuite) TestAuthenticateFailsWithNonExistentUser(c *gc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{
		Name:        "bobbrown",
		DisplayName: "Bob Brown",
		Password:    "password",
	})

	s.username = "notbobbrown"

	svc, err := bakery.NewService(bakery.NewServiceParams{
		Locator: s.discharger,
	})
	c.Assert(err, jc.ErrorIsNil)
	mac, err := svc.NewMacaroon("", nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	authenticator := &authentication.MacaroonAuthenticator{
		Service:          svc,
		IdentityLocation: s.discharger.Location(),
		Macaroon:         mac,
	}
	_, err = authenticator.Authenticate(nil, user.Tag(), params.LoginRequest{})
	dischargeErr := err.(*authentication.DischargeRequiredError)
	client := httpbakery.NewClient()
	ms, err := client.DischargeAll(dischargeErr.Macaroon)
	c.Assert(err, jc.ErrorIsNil)
	_, err = authenticator.Authenticate(s.State, user.Tag(), params.LoginRequest{
		Credentials: "",
		Nonce:       "",
		Macaroons:   ms,
	})
	c.Assert(err, gc.ErrorMatches, "invalid entity name or password")
}

func (s *macaroonAuthenticatorSuite) TestInvalidUserName(c *gc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{
		Name:        "bobbrown",
		DisplayName: "Bob Brown",
		Password:    "password",
	})

	s.username = "--"

	svc, err := bakery.NewService(bakery.NewServiceParams{
		Locator: s.discharger,
	})
	c.Assert(err, jc.ErrorIsNil)
	mac, err := svc.NewMacaroon("", nil, nil)
	c.Assert(err, jc.ErrorIsNil)
	authenticator := &authentication.MacaroonAuthenticator{
		Service:          svc,
		IdentityLocation: s.discharger.Location(),
		Macaroon:         mac,
	}
	_, err = authenticator.Authenticate(nil, user.Tag(), params.LoginRequest{
		Credentials: "",
		Nonce:       "",
		Macaroons:   nil,
	})
	dischargeErr := err.(*authentication.DischargeRequiredError)
	client := httpbakery.NewClient()
	ms, err := client.DischargeAll(dischargeErr.Macaroon)
	c.Assert(err, jc.ErrorIsNil)
	entity, err := authenticator.Authenticate(s.State, user.Tag(), params.LoginRequest{
		Credentials: "",
		Nonce:       "",
		Macaroons:   ms,
	})
	c.Assert(err, gc.ErrorMatches, `"--" is an invalid user name`)
	c.Assert(entity, gc.IsNil)
}
