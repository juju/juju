// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gceapi

import (
	"code.google.com/p/goauth2/oauth"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/config"
)

type authSuite struct {
	baseSuite
}

var _ = gc.Suite(&authSuite{})

func (*authSuite) TestAuthValidate(c *gc.C) {
	auth := Auth{
		ClientID:    "spam",
		ClientEmail: "user@mail.com",
		PrivateKey:  []byte("non-empty"),
	}
	err := auth.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (*authSuite) TestAuthValidateMissingID(c *gc.C) {
	auth := Auth{
		ClientEmail: "user@mail.com",
		PrivateKey:  []byte("non-empty"),
	}
	err := auth.Validate()

	c.Assert(err, gc.FitsTypeOf, &config.InvalidConfigValue{})
	c.Check(err.(*config.InvalidConfigValue).Key, gc.Equals, "GCE_CLIENT_ID")
}

func (*authSuite) TestAuthValidateBadEmail(c *gc.C) {
	auth := Auth{
		ClientID:    "spam",
		ClientEmail: "bad_email",
		PrivateKey:  []byte("non-empty"),
	}
	err := auth.Validate()

	c.Assert(err, gc.FitsTypeOf, &config.InvalidConfigValue{})
	c.Check(err.(*config.InvalidConfigValue).Key, gc.Equals, "GCE_CLIENT_EMAIL")
}

func (*authSuite) TestAuthValidateMissingKey(c *gc.C) {
	auth := Auth{
		ClientID:    "spam",
		ClientEmail: "user@mail.com",
	}
	err := auth.Validate()

	c.Assert(err, gc.FitsTypeOf, &config.InvalidConfigValue{})
	c.Check(err.(*config.InvalidConfigValue).Key, gc.Equals, "GCE_PRIVATE_KEY")
}

func (s *authSuite) TestAuthNewTransport(c *gc.C) {
	token := &oauth.Token{}
	s.patchNewToken(c, s.auth, "", token)
	transport, err := s.auth.newTransport()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(transport.Config.ClientId, gc.Equals, "spam")
	c.Check(transport.Config.Scope, gc.Equals, "https://www.googleapis.com/auth/compute https://www.googleapis.com/auth/devstorage.full_control")
	c.Check(transport.Config.TokenURL, gc.Equals, "https://accounts.google.com/o/oauth2/token")
	c.Check(transport.Config.AuthURL, gc.Equals, "https://accounts.google.com/o/oauth2/auth")
	c.Check(transport.Token, gc.Equals, token)
}

// Testing the newToken valid case would require valid credentials, so
// we don't bother.

func (s *authSuite) TestAuthNewTokenBadCredentials(c *gc.C) {
	// Makes an HTTP request to the GCE API.
	_, err := newToken(s.auth, "")

	c.Check(errors.Cause(err), gc.ErrorMatches, "Invalid Key")
}

func (s *authSuite) TestAuthNewConnection(c *gc.C) {
	s.patchNewToken(c, s.auth, "", nil)
	service, err := s.auth.newConnection()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(service, gc.NotNil)
}

func (s *authSuite) TestAuthNewService(c *gc.C) {
	s.patchNewToken(c, s.auth, "", nil)

	transport, err := s.auth.newTransport()
	c.Assert(err, jc.ErrorIsNil)
	service, err := newService(transport)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(service, gc.NotNil)
}
