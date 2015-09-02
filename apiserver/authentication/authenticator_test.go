// Copyright 2014 Canonical Ltd. All rights reserved.
// Licensed under the AGPLv3, see LICENCE file for details.

package authentication_test

import (
	"github.com/juju/names"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state"
	"github.com/juju/juju/testing/factory"
)

type AgentAuthenticatorSuite struct {
	testing.JujuConnSuite
}

type userFinder struct {
	user state.Entity
}

func (u userFinder) FindEntity(tag names.Tag) (state.Entity, error) {
	return u.user, nil
}

func (s *AgentAuthenticatorSuite) TestAuthenticatorForTagFails(c *gc.C) {
	// add relation
	wordpress := s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	wordpressEP, err := wordpress.Endpoint("db")
	c.Assert(err, jc.ErrorIsNil)
	mysql := s.AddTestingService(c, "mysql", s.AddTestingCharm(c, "mysql"))
	mysqlEP, err := mysql.Endpoint("server")
	c.Assert(err, jc.ErrorIsNil)
	relation, err := s.State.AddRelation(wordpressEP, mysqlEP)
	c.Assert(err, jc.ErrorIsNil)

	_, err = authentication.AuthenticatorForTag(relation.String())
	c.Assert(err, gc.ErrorMatches, `failed to determine the tag kind: "wordpress:db mysql:server" is not a valid tag`)
}

func (s *AgentAuthenticatorSuite) TestAuthenticatorForTag(c *gc.C) {
	fact := factory.NewFactory(s.State)
	user := fact.MakeUser(c, &factory.UserParams{Password: "password"})
	authenticator, err := authentication.AuthenticatorForTag(user.Tag().String())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(authenticator, gc.NotNil)
	userFinder := userFinder{user}

	entity, err := authenticator.Authenticate(userFinder, user.Tag(), params.LoginRequest{
		Credentials: "password",
		Nonce:       "nonce",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(entity, gc.DeepEquals, user)
}

func (s *AgentAuthenticatorSuite) TestAuthenticatorForTagGetsMacaroonAuthenticator(c *gc.C) {
	authenticator, err := authentication.AuthenticatorForTag("")
	c.Assert(err, jc.ErrorIsNil)
	_, ok := authenticator.(*authentication.MacaroonAuthenticator)
	c.Assert(ok, jc.IsTrue)
}

func (s *AgentAuthenticatorSuite) TestMachineGetsAgentAuthentictor(c *gc.C) {
	authenticator, err := authentication.AuthenticatorForTag("machine-0")
	c.Assert(err, jc.ErrorIsNil)
	_, ok := authenticator.(*authentication.AgentAuthenticator)
	c.Assert(ok, jc.IsTrue)
}
func (s *AgentAuthenticatorSuite) TestUnitGetsAgentAuthentictor(c *gc.C) {
	authenticator, err := authentication.AuthenticatorForTag("unit-wordpress")
	c.Assert(err, jc.ErrorIsNil)
	_, ok := authenticator.(*authentication.AgentAuthenticator)
	c.Assert(ok, jc.IsTrue)
}
func (s *AgentAuthenticatorSuite) TestInvalidTag(c *gc.C) {
	authenticator, err := authentication.AuthenticatorForTag("service-foobar")
	c.Assert(err, gc.ErrorMatches, "invalid request")
	c.Assert(authenticator, gc.IsNil)
}
