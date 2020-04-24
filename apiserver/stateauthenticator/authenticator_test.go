// Copyright 2014-2018 Canonical Ltd. All rights reserved.
// Licensed under the AGPLv3, see LICENCE file for details.

package stateauthenticator_test

import (
	"context"

	"github.com/juju/clock"
	"github.com/juju/names/v4"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/params"
	"github.com/juju/juju/apiserver/stateauthenticator"
	"github.com/juju/juju/state"
	statetesting "github.com/juju/juju/state/testing"
	"github.com/juju/juju/testing/factory"
)

// TODO update these tests (moved from apiserver) to test
// via the public interface, and then get rid of export_test.go.
type agentAuthenticatorSuite struct {
	statetesting.StateSuite
	authenticator *stateauthenticator.Authenticator
}

var _ = gc.Suite(&agentAuthenticatorSuite{})

func (s *agentAuthenticatorSuite) SetUpTest(c *gc.C) {
	s.StateSuite.SetUpTest(c)
	authenticator, err := stateauthenticator.NewAuthenticator(s.StatePool, clock.WallClock)
	c.Assert(err, jc.ErrorIsNil)
	s.authenticator = authenticator
}

func (s *agentAuthenticatorSuite) TestAuthenticatorForTag(c *gc.C) {
	user := s.Factory.MakeUser(c, &factory.UserParams{Password: "password"})

	authenticator, err := stateauthenticator.EntityAuthenticator(s.authenticator, user.Tag())
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(authenticator, gc.NotNil)
	userFinder := userFinder{user}

	entity, err := authenticator.Authenticate(context.TODO(), userFinder, user.Tag(), params.LoginRequest{
		Credentials: "password",
		Nonce:       "nonce",
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(entity, gc.DeepEquals, user)
}

func (s *agentAuthenticatorSuite) TestMachineGetsAgentAuthenticator(c *gc.C) {
	authenticator, err := stateauthenticator.EntityAuthenticator(s.authenticator, names.NewMachineTag("0"))
	c.Assert(err, jc.ErrorIsNil)
	_, ok := authenticator.(*authentication.AgentAuthenticator)
	c.Assert(ok, jc.IsTrue)
}

func (s *agentAuthenticatorSuite) TestUnitGetsAgentAuthenticator(c *gc.C) {
	authenticator, err := stateauthenticator.EntityAuthenticator(s.authenticator, names.NewUnitTag("wordpress/0"))
	c.Assert(err, jc.ErrorIsNil)
	_, ok := authenticator.(*authentication.AgentAuthenticator)
	c.Assert(ok, jc.IsTrue)
}

func (s *agentAuthenticatorSuite) TestNotSupportedTag(c *gc.C) {
	authenticator, err := stateauthenticator.EntityAuthenticator(s.authenticator, names.NewCloudTag("not-support"))
	c.Assert(err, gc.ErrorMatches, "unexpected login entity tag: invalid request")
	c.Assert(authenticator, gc.IsNil)
}

type userFinder struct {
	user state.Entity
}

func (u userFinder) FindEntity(tag names.Tag) (state.Entity, error) {
	return u.user, nil
}
