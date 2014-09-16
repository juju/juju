// Copyright 2014 Canonical Ltd. All rights reserved.
// Licensed under the AGPLv3, see LICENCE file for details.

package authentication_test

import (
	gc "launchpad.net/gocheck"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type AgentAuthenticatorSuite struct {
	testing.JujuConnSuite
}

func (s *AgentAuthenticatorSuite) TestFindEntityAuthenticatorFails(c *gc.C) {
	// add relation
	wordpress := s.AddTestingService(c, "wordpress", s.AddTestingCharm(c, "wordpress"))
	wordpressEP, err := wordpress.Endpoint("db")
	c.Assert(err, gc.IsNil)
	mysql := s.AddTestingService(c, "mysql", s.AddTestingCharm(c, "mysql"))
	mysqlEP, err := mysql.Endpoint("server")
	c.Assert(err, gc.IsNil)
	relation, err := s.State.AddRelation(wordpressEP, mysqlEP)
	c.Assert(err, gc.IsNil)

	_, err = authentication.FindEntityAuthenticator(relation)
	c.Assert(err, gc.ErrorMatches, "invalid request")
}

func (s *AgentAuthenticatorSuite) TestFindEntityAuthenticator(c *gc.C) {
	fact := factory.NewFactory(s.State)
	user := fact.MakeUser(c, &factory.UserParams{Password: "password"})
	authenticator, err := authentication.FindEntityAuthenticator(user)
	c.Assert(err, gc.IsNil)
	c.Assert(authenticator, gc.NotNil)

	err = authenticator.Authenticate(user, "password", "nonce")
	c.Assert(err, gc.IsNil)
}
