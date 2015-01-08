// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package gceapi

import (
	"code.google.com/p/goauth2/oauth"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/environs/config"
	"github.com/juju/juju/testing"
)

type gceSuite struct {
	testing.BaseSuite

	auth gceAuth
}

var _ = gc.Suite(&gceSuite{})

func (s *gceSuite) SetUpTest(c *gc.C) {
	s.BaseSuite.SetUpTest(c)
	s.auth = gceAuth{
		clientID:    "spam",
		clientEmail: "user@mail.com",
		privateKey:  []byte("non-empty"),
	}
}

func (s *gceSuite) patchNewToken(c *gc.C, expectedAuth gceAuth, expectedScopes string, token *oauth.Token) {
	if expectedScopes == "" {
		expectedScopes = "https://www.googleapis.com/auth/compute https://www.googleapis.com/auth/devstorage.full_control"
	}
	if token == nil {
		token = &oauth.Token{}
	}
	s.PatchValue(&newToken, func(auth gceAuth, scopes string) (*oauth.Token, error) {
		c.Check(auth, jc.DeepEquals, expectedAuth)
		c.Check(scopes, gc.Equals, expectedScopes)
		return token, nil
	})
}

func (*gceSuite) TestAuthValidate(c *gc.C) {
	auth := gceAuth{
		clientID:    "spam",
		clientEmail: "user@mail.com",
		privateKey:  []byte("non-empty"),
	}
	err := auth.validate()

	c.Check(err, jc.ErrorIsNil)
}

func (*gceSuite) TestAuthValidateMissingID(c *gc.C) {
	auth := gceAuth{
		clientEmail: "user@mail.com",
		privateKey:  []byte("non-empty"),
	}
	err := auth.validate()

	c.Assert(err, gc.FitsTypeOf, &config.InvalidConfigValue{})
	c.Check(err.(*config.InvalidConfigValue).Key, gc.Equals, "GCE_CLIENT_ID")
}

func (*gceSuite) TestAuthValidateBadEmail(c *gc.C) {
	auth := gceAuth{
		clientID:    "spam",
		clientEmail: "bad_email",
		privateKey:  []byte("non-empty"),
	}
	err := auth.validate()

	c.Assert(err, gc.FitsTypeOf, &config.InvalidConfigValue{})
	c.Check(err.(*config.InvalidConfigValue).Key, gc.Equals, "GCE_CLIENT_EMAIL")
}

func (*gceSuite) TestAuthValidateMissingKey(c *gc.C) {
	auth := gceAuth{
		clientID:    "spam",
		clientEmail: "user@mail.com",
	}
	err := auth.validate()

	c.Assert(err, gc.FitsTypeOf, &config.InvalidConfigValue{})
	c.Check(err.(*config.InvalidConfigValue).Key, gc.Equals, "GCE_PRIVATE_KEY")
}

func (s *gceSuite) TestAuthNewTransport(c *gc.C) {
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

func (s *gceSuite) TestAuthNewTokenBadCredentials(c *gc.C) {
	// Makes an HTTP request to the GCE API.
	_, err := newToken(s.auth, "")

	c.Check(errors.Cause(err), gc.ErrorMatches, "Invalid Key")
}

func (s *gceSuite) TestAuthNewConnection(c *gc.C) {
	s.patchNewToken(c, s.auth, "", nil)
	service, err := s.auth.newConnection()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(service, gc.NotNil)
}

func (s *gceSuite) TestAuthNewService(c *gc.C) {
	s.patchNewToken(c, s.auth, "", nil)

	transport, err := s.auth.newTransport()
	c.Assert(err, jc.ErrorIsNil)
	service, err := newService(transport)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(service, gc.NotNil)
}
