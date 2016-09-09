// Copyright 2014 Canonical Ltd. All rights reserved.
// Licensed under the AGPLv3, see LICENCE file for details.

package authentication_test

import (
	"net/http"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/testing"
	jc "github.com/juju/testing/checkers"
	"github.com/juju/utils"
	gc "gopkg.in/check.v1"
	"gopkg.in/juju/names.v2"
	"gopkg.in/macaroon-bakery.v1/bakery"
	"gopkg.in/macaroon-bakery.v1/bakery/checkers"
	"gopkg.in/macaroon-bakery.v1/bakerytest"
	"gopkg.in/macaroon-bakery.v1/httpbakery"
	"gopkg.in/macaroon.v1"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/common"
	"github.com/juju/juju/apiserver/params"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
)

var logger = loggo.GetLogger("juju.apiserver.authentication")

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

func (s *userAuthenticatorSuite) TestValidMacaroonUserLogin(c *gc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{
		Name: "bobbrown",
	})
	macaroons := []macaroon.Slice{{&macaroon.Macaroon{}}}
	service := mockBakeryService{}

	// User login
	authenticator := &authentication.UserAuthenticator{Service: &service}
	_, err := authenticator.Authenticate(s.State, user.Tag(), params.LoginRequest{
		Credentials: "",
		Nonce:       "",
		Macaroons:   macaroons,
	})
	c.Assert(err, jc.ErrorIsNil)

	service.CheckCallNames(c, "CheckAny")
	call := service.Calls()[0]
	c.Assert(call.Args, gc.HasLen, 3)
	c.Assert(call.Args[0], jc.DeepEquals, macaroons)
	c.Assert(call.Args[1], jc.DeepEquals, map[string]string{"username": "bobbrown@local"})
	// no check for checker function, can't compare functions
}

func (s *userAuthenticatorSuite) TestCreateLocalLoginMacaroon(c *gc.C) {
	service := mockBakeryService{}
	clock := testing.NewClock(time.Time{})
	_, err := authentication.CreateLocalLoginMacaroon(
		names.NewUserTag("bobbrown"), &service, clock,
	)
	c.Assert(err, jc.ErrorIsNil)
	service.CheckCallNames(c, "NewMacaroon")
	service.CheckCall(c, 0, "NewMacaroon", "", []byte(nil), []checkers.Caveat{
		{Condition: "is-authenticated-user bobbrown@local"},
		{Condition: "time-before 0001-01-01T00:02:00Z"},
	})
}

func (s *userAuthenticatorSuite) TestAuthenticateLocalLoginMacaroon(c *gc.C) {
	service := mockBakeryService{}
	clock := testing.NewClock(time.Time{})
	authenticator := &authentication.UserAuthenticator{
		Service: &service,
		Clock:   clock,
		LocalUserIdentityLocation: "https://testing.invalid:1234/auth",
	}

	service.SetErrors(&bakery.VerificationError{})
	_, err := authenticator.Authenticate(
		authentication.EntityFinder(nil),
		names.NewUserTag("bobbrown"),
		params.LoginRequest{},
	)
	c.Assert(err, gc.FitsTypeOf, &common.DischargeRequiredError{})

	service.CheckCallNames(c, "CheckAny", "ExpireStorageAt", "NewMacaroon")
	calls := service.Calls()
	c.Assert(calls[1].Args, jc.DeepEquals, []interface{}{clock.Now().Add(24 * time.Hour)})
	c.Assert(calls[2].Args, jc.DeepEquals, []interface{}{
		"", []byte(nil), []checkers.Caveat{
			checkers.NeedDeclaredCaveat(
				checkers.Caveat{
					Location:  "https://testing.invalid:1234/auth",
					Condition: "is-authenticated-user bobbrown@local",
				},
				"username",
			),
			{Condition: "time-before 0001-01-02T00:00:00Z"},
		},
	})
}

type mockBakeryService struct {
	testing.Stub
}

func (s *mockBakeryService) AddCaveat(m *macaroon.Macaroon, caveat checkers.Caveat) error {
	s.MethodCall(s, "AddCaveat", m, caveat)
	return s.NextErr()
}

func (s *mockBakeryService) CheckAny(ms []macaroon.Slice, assert map[string]string, checker checkers.Checker) (map[string]string, error) {
	s.MethodCall(s, "CheckAny", ms, assert, checker)
	return nil, s.NextErr()
}

func (s *mockBakeryService) NewMacaroon(id string, key []byte, caveats []checkers.Caveat) (*macaroon.Macaroon, error) {
	s.MethodCall(s, "NewMacaroon", id, key, caveats)
	return &macaroon.Macaroon{}, s.NextErr()
}

func (s *mockBakeryService) ExpireStorageAt(t time.Time) (authentication.ExpirableStorageBakeryService, error) {
	s.MethodCall(s, "ExpireStorageAt", t)
	return s, s.NextErr()
}

type macaroonAuthenticatorSuite struct {
	jujutesting.JujuConnSuite
	// username holds the username that will be
	// declared in the discharger's caveats.
	username string
}

var _ = gc.Suite(&macaroonAuthenticatorSuite{})

func (s *macaroonAuthenticatorSuite) Checker(req *http.Request, cond, arg string) ([]checkers.Caveat, error) {
	return []checkers.Caveat{checkers.DeclaredCaveat("username", s.username)}, nil
}

var authenticateSuccessTests = []struct {
	about              string
	dischargedUsername string
	finder             authentication.EntityFinder
	expectTag          string
	expectError        string
}{{
	about:              "user that can be found",
	dischargedUsername: "bobbrown@somewhere",
	expectTag:          "user-bobbrown@somewhere",
	finder: simpleEntityFinder{
		"user-bobbrown@somewhere": true,
	},
}, {
	about:              "user with no @ domain",
	dischargedUsername: "bobbrown",
	finder: simpleEntityFinder{
		"user-bobbrown@external": true,
	},
	expectTag: "user-bobbrown@external",
}, {
	about:              "user not found in database",
	dischargedUsername: "bobbrown@nowhere",
	finder:             simpleEntityFinder{},
	expectError:        "invalid entity name or password",
}, {
	about:              "invalid user name",
	dischargedUsername: "--",
	finder:             simpleEntityFinder{},
	expectError:        `"--" is an invalid user name`,
}, {
	about:              "ostensibly local name",
	dischargedUsername: "cheat@local",
	finder: simpleEntityFinder{
		"cheat@local": true,
	},
	expectError: `external identity provider has provided ostensibly local name "cheat@local"`,
}, {
	about:              "FindEntity error",
	dischargedUsername: "bobbrown@nowhere",
	finder:             errorEntityFinder("lost in space"),
	expectError:        "lost in space",
}}

func (s *macaroonAuthenticatorSuite) TestMacaroonAuthentication(c *gc.C) {
	discharger := bakerytest.NewDischarger(nil, s.Checker)
	defer discharger.Close()
	for i, test := range authenticateSuccessTests {
		c.Logf("\ntest %d; %s", i, test.about)
		s.username = test.dischargedUsername

		svc, err := bakery.NewService(bakery.NewServiceParams{
			Locator: discharger,
		})
		c.Assert(err, jc.ErrorIsNil)
		mac, err := svc.NewMacaroon("", nil, nil)
		c.Assert(err, jc.ErrorIsNil)
		authenticator := &authentication.ExternalMacaroonAuthenticator{
			Service:          svc,
			IdentityLocation: discharger.Location(),
			Macaroon:         mac,
		}

		// Authenticate once to obtain the macaroon to be discharged.
		_, err = authenticator.Authenticate(test.finder, nil, params.LoginRequest{
			Credentials: "",
			Nonce:       "",
			Macaroons:   nil,
		})

		// Discharge the macaroon.
		dischargeErr := errors.Cause(err).(*common.DischargeRequiredError)
		client := httpbakery.NewClient()
		ms, err := client.DischargeAll(dischargeErr.Macaroon)
		c.Assert(err, jc.ErrorIsNil)

		// Authenticate again with the discharged macaroon.
		entity, err := authenticator.Authenticate(test.finder, nil, params.LoginRequest{
			Credentials: "",
			Nonce:       "",
			Macaroons:   []macaroon.Slice{ms},
		})
		if test.expectError != "" {
			c.Assert(err, gc.ErrorMatches, test.expectError)
			c.Assert(entity, gc.Equals, nil)
		} else {
			c.Assert(err, jc.ErrorIsNil)
			c.Assert(entity.Tag().String(), gc.Equals, test.expectTag)
		}
	}
}

type errorEntityFinder string

func (f errorEntityFinder) FindEntity(tag names.Tag) (state.Entity, error) {
	return nil, errors.New(string(f))
}

type simpleEntityFinder map[string]bool

func (f simpleEntityFinder) FindEntity(tag names.Tag) (state.Entity, error) {
	if utag, ok := tag.(names.UserTag); ok {
		// It's a user tag which we need to be in canonical form
		// so we can look it up unambiguously.
		tag = names.NewUserTag(utag.Canonical())
	}
	if f[tag.String()] {
		return &simpleEntity{tag}, nil
	}
	return nil, errors.NotFoundf("entity %q", tag)
}

type simpleEntity struct {
	tag names.Tag
}

func (e *simpleEntity) Tag() names.Tag {
	return e.tag
}
