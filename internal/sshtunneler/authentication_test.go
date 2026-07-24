// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshtunneler

import (
	"encoding/base64"
	"testing"
	"time"

	gomock "github.com/canonical/gomock/gomock"
	"github.com/juju/tc"
	"github.com/lestrrat-go/jwx/v3/jwt"
)

type authenticationSuite struct {
	clock *MockClock
}

func TestAuthenticationSuite(t *testing.T) {
	tc.Run(t, &authenticationSuite{})
}

func (s *authenticationSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.clock = NewMockClock(ctrl)

	return ctrl
}

func (s *authenticationSuite) newAuthn(c *tc.C) tunnelAuthentication {
	authn, err := newTunnelAuthentication(s.clock)
	c.Assert(err, tc.ErrorIsNil)
	return authn
}

func (s *authenticationSuite) TestGeneratePassword(c *tc.C) {
	defer s.setupMocks(c).Finish()

	authn := s.newAuthn(c)

	now := time.Now()
	deadline := now.Add(maxTimeout)

	tunnelID := "test-tunnel-id"
	token, err := authn.generatePassword(tunnelID, now, deadline)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(token, tc.Not(tc.Equals), "")

	rawToken, err := base64.StdEncoding.DecodeString(token)
	c.Assert(err, tc.ErrorIsNil)

	s.clock.EXPECT().Now().AnyTimes().Return(now)

	parsedToken, err := jwt.Parse(rawToken, jwt.WithKey(authn.jwtAlg, authn.sharedSecret), jwt.WithClock(s.clock))
	c.Assert(err, tc.ErrorIsNil)
	subject, ok := parsedToken.Subject()
	c.Assert(ok, tc.IsTrue)
	c.Assert(subject, tc.Equals, tokenSubject)
	issuer, ok := parsedToken.Issuer()
	c.Assert(ok, tc.IsTrue)
	c.Assert(issuer, tc.Equals, tokenIssuer)
	expiration, ok := parsedToken.Expiration()
	c.Assert(ok, tc.IsTrue)
	issuedAt, ok := parsedToken.IssuedAt()
	c.Assert(ok, tc.IsTrue)
	c.Assert(expiration.Sub(issuedAt), tc.Equals, maxTimeout)
	var tokTunnelID string
	err = parsedToken.Get(tunnelIDClaimKey, &tokTunnelID)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(tokTunnelID, tc.Equals, tunnelID)
}

func (s *authenticationSuite) TestValidatePasswordInvalidToken(c *tc.C) {
	defer s.setupMocks(c).Finish()

	authn := s.newAuthn(c)

	_, err := authn.validatePassword("invalid-token")
	c.Assert(err, tc.ErrorMatches, "failed to decode token: .*")
}

func (s *authenticationSuite) TestValidatePasswordExpiredToken(c *tc.C) {
	defer s.setupMocks(c).Finish()

	authn := s.newAuthn(c)

	now := time.Now()
	deadline := now.Add(maxTimeout)

	tunnelID := "test-tunnel-id"
	token, err := authn.generatePassword(tunnelID, now, deadline)
	c.Assert(err, tc.ErrorIsNil)

	expiry := now.Add(maxTimeout)
	s.clock.EXPECT().Now().AnyTimes().Return(expiry)

	_, err = authn.validatePassword(token)
	c.Assert(err, tc.ErrorMatches, `.*"exp" not satisfied: token is expired`)
}
