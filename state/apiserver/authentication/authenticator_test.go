// Copyright 2014 Canonical Ltd. All rights reserved.
// Licensed under the AGPLv3, see LICENCE file for details.

package authentication_test

import (
	gc "launchpad.net/gocheck"
	"testing"

	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/state/apiserver/authentication"
	coretesting "github.com/juju/juju/testing"
	"github.com/juju/juju/testing/factory"
)

type AgentAuthenticatorSuite struct {
	jujutesting.JujuConnSuite
}

var _ = gc.Suite(&AgentAuthenticatorSuite{})

func TestAll(t *testing.T) {
	coretesting.MgoTestPackage(t)
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
	c.Assert(err, gc.ErrorMatches, "entity with tag type 'relation' does not have an authenticator")
}

func (s *AgentAuthenticatorSuite) TestFindEntityAuthenticator(c *gc.C) {
	fact := factory.NewFactory(s.State, c)
	provider, err := authentication.FindEntityAuthenticator(fact.MakeAnyUser())
	c.Assert(err, gc.IsNil)
	c.Assert(provider, gc.NotNil)
}
