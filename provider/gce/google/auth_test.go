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

func (s *authSuite) TestAuthNewTransport(c *gc.C) {
	token := &oauth.Token{}
	s.patchNewToken(c, s.Auth, "", token)
	transport, err := s.Auth.newTransport()
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
	_, err := newToken(s.Auth, "")

	c.Check(errors.Cause(err), gc.ErrorMatches, "Invalid Key")
}

func (s *authSuite) TestAuthNewConnection(c *gc.C) {
	s.patchNewToken(c, s.Auth, "", nil)
	service, err := s.Auth.newConnection()
	c.Assert(err, jc.ErrorIsNil)

	c.Check(service, gc.NotNil)
}

func (s *authSuite) TestAuthNewService(c *gc.C) {
	s.patchNewToken(c, s.Auth, "", nil)

	transport, err := s.Auth.newTransport()
	c.Assert(err, jc.ErrorIsNil)
	service, err := newService(transport)
	c.Assert(err, jc.ErrorIsNil)

	c.Check(service, gc.NotNil)
}
