// Copyright 2014 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package google

import (
	"code.google.com/p/goauth2/oauth"
	"github.com/juju/errors"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"
)

type authSuite struct {
	BaseSuite
}

var _ = gc.Suite(&authSuite{})

func (s *authSuite) TestNewTransport(c *gc.C) {
	token := &oauth.Token{}
	s.patchNewToken(c, s.Credentials, "", token)
	transport, err := newTransport(s.Credentials)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(transport.Config.ClientId, gc.Equals, "spam")
	c.Check(transport.Config.Scope, gc.Equals, "https://www.googleapis.com/auth/compute https://www.googleapis.com/auth/devstorage.full_control")
	c.Check(transport.Config.TokenURL, gc.Equals, "https://accounts.google.com/o/oauth2/token")
	c.Check(transport.Config.AuthURL, gc.Equals, "https://accounts.google.com/o/oauth2/auth")
	c.Check(transport.Token, gc.Equals, token)
}

// Testing the newToken valid case would require valid credentials, so
// we don't bother.

func (s *authSuite) TestNewTokenBadCredentials(c *gc.C) {
	// Makes an HTTP request to the GCE API.
	_, err := newToken(s.Credentials, "")

	c.Check(errors.Cause(err), gc.ErrorMatches, "Invalid Key")
}

func (s *authSuite) TestNewConnection(c *gc.C) {
	s.patchNewToken(c, s.Credentials, "", nil)
	service, err := newConnection(s.Credentials)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(service, gc.NotNil)
}

func (s *authSuite) TestNewService(c *gc.C) {
	s.patchNewToken(c, s.Credentials, "", nil)

	transport, err := newTransport(s.Credentials)
	c.Assert(err, jc.ErrorIsNil)
	service, err := newService(transport)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(service, gc.NotNil)
}

type credentialsSuite struct {
	BaseSuite
}

var _ = gc.Suite(&credentialsSuite{})

func (*credentialsSuite) TestValidateValid(c *gc.C) {
	creds := &Credentials{
		ClientID:    "spam",
		ClientEmail: "user@mail.com",
		PrivateKey:  []byte("non-empty"),
	}
	err := creds.Validate()

	c.Check(err, jc.ErrorIsNil)
}

func (*credentialsSuite) TestValidateMissingID(c *gc.C) {
	creds := &Credentials{
		ClientEmail: "user@mail.com",
		PrivateKey:  []byte("non-empty"),
	}
	err := creds.Validate()

	c.Assert(err, gc.FitsTypeOf, &InvalidCredential{})
	c.Check(err.(*InvalidCredential).Key, gc.Equals, "GCE_CLIENT_ID")
}

func (*credentialsSuite) TestValidateBadEmail(c *gc.C) {
	creds := &Credentials{
		ClientID:    "spam",
		ClientEmail: "bad_email",
		PrivateKey:  []byte("non-empty"),
	}
	err := creds.Validate()

	c.Assert(err, gc.FitsTypeOf, &InvalidCredential{})
	c.Check(err.(*InvalidCredential).Key, gc.Equals, "GCE_CLIENT_EMAIL")
}

func (*credentialsSuite) TestValidateMissingKey(c *gc.C) {
	creds := &Credentials{
		ClientID:    "spam",
		ClientEmail: "user@mail.com",
	}
	err := creds.Validate()

	c.Assert(err, gc.FitsTypeOf, &InvalidCredential{})
	c.Check(err.(*InvalidCredential).Key, gc.Equals, "GCE_PRIVATE_KEY")
}
