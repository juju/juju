// Copyright 2014 Canonical Ltd. All rights reserved.
// Licensed under the AGPLv3, see LICENCE file for details.

package authentication_test

import (
	"context"

	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/core/permission"
	usertesting "github.com/juju/juju/core/user/testing"
	"github.com/juju/juju/domain/access/service"
	"github.com/juju/juju/internal/auth"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	internalpassword "github.com/juju/juju/internal/password"
	"github.com/juju/juju/internal/testing/factory"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
)

type agentAuthenticatorSuite struct {
	testing.ApiServerSuite
	machinePassword string
	machineNonce    string
	unitPassword    string
	machine         *state.Machine
	user            state.Entity
	unit            *state.Unit
	relation        *state.Relation
}

var _ = gc.Suite(&agentAuthenticatorSuite{})

func (s *agentAuthenticatorSuite) SetUpTest(c *gc.C) {
	s.ApiServerSuite.SetUpTest(c)

	userService := s.ControllerDomainServices(c).Access()
	userUUID, _, err := userService.AddUser(context.Background(), service.AddUserArg{
		Name:        usertesting.GenNewName(c, "bobbrown"),
		DisplayName: "Bob Brown",
		Password:    ptr(auth.NewPassword("password")),
		CreatorUUID: s.AdminUserUUID,
		Permission: permission.AccessSpec{
			Access: permission.LoginAccess,
			Target: permission.ID{
				ObjectType: permission.Controller,
				Key:        s.ControllerUUID,
			},
		},
	})
	c.Assert(err, jc.ErrorIsNil)
	user, err := userService.GetUser(context.Background(), userUUID)
	c.Assert(err, jc.ErrorIsNil)
	s.user = authentication.TaggedUser(user, names.NewUserTag("bobbrown"))

	modelConfigService := s.ControllerDomainServices(c).Config()
	f, release := s.NewFactory(c, s.ControllerModelUUID())
	defer release()
	f = f.WithModelConfigService(modelConfigService)

	// add machine for testing machine agent authentication
	st := s.ControllerModel(c).State()
	machine, err := st.AddMachine(modelConfigService, state.UbuntuBase("12.10"), state.JobHostUnits)
	c.Assert(err, jc.ErrorIsNil)
	nonce, err := internalpassword.RandomPassword()
	c.Assert(err, jc.ErrorIsNil)
	err = machine.SetProvisioned("foo", "", nonce, nil)
	c.Assert(err, jc.ErrorIsNil)
	password, err := internalpassword.RandomPassword()
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
	unit, err := wordpress.AddUnit(modelConfigService, state.AddUnitParams{})
	c.Assert(err, jc.ErrorIsNil)
	s.unit = unit
	password, err = internalpassword.RandomPassword()
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
		factory := authentication.NewAgentAuthenticatorFactory(
			st,
			loggertesting.WrapCheckLog(c),
		)
		entity, err := factory.Authenticator().Authenticate(context.Background(), authentication.AuthParams{
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
		entity:       s.user,
		credentials:  "password",
		about:        "user login",
		errorMessage: "invalid request",
	}, {
		entity:       s.relation,
		credentials:  "dummy-secret",
		about:        "relation login",
		errorMessage: "invalid request",
	}, {
		entity:       s.user,
		credentials:  "wrongpassword",
		about:        "user login for nonexistant user",
		errorMessage: "invalid request",
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
		errorMessage: "invalid request",
	}}

	st := s.ControllerModel(c).State()
	for i, t := range testCases {
		c.Logf("test %d: %s", i, t.about)
		factory := authentication.NewAgentAuthenticatorFactory(
			st,
			loggertesting.WrapCheckLog(c),
		)
		entity, err := factory.Authenticator().Authenticate(context.Background(), authentication.AuthParams{
			AuthTag:     t.entity.Tag(),
			Credentials: t.credentials,
			Nonce:       t.nonce,
		})
		c.Assert(err, gc.ErrorMatches, t.errorMessage)
		c.Assert(entity, gc.IsNil)
	}
}
