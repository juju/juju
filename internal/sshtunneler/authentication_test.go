// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshtunneler

import (
	"encoding/base64"
	"time"

	jc "github.com/juju/testing/checkers"
	"github.com/lestrrat-go/jwx/v2/jwt"
	gomock "go.uber.org/mock/gomock"
	gc "gopkg.in/check.v1"
)

type authenticationSuite struct {
	clock *MockClock
}

var _ = gc.Suite(&authenticationSuite{})

func (s *authenticationSuite) setupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)

	s.clock = NewMockClock(ctrl)

	return ctrl
}

func (s *authenticationSuite) newAuthn(c *gc.C) tunnelAuthentication {
	authn, err := newTunnelAuthentication(s.clock)
	c.Assert(err, jc.ErrorIsNil)
	return authn
}

func (s *authenticationSuite) TestGeneratePassword(c *gc.C) {
	defer s.setupMocks(c).Finish()

	authn := s.newAuthn(c)

	now := time.Now()
	deadline := now.Add(maxTimeout)

	tunnelID := "test-tunnel-id"
	token, err := authn.generatePassword(tunnelID, now, deadline)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(token, gc.Not(gc.Equals), "")

	rawToken, err := base64.StdEncoding.DecodeString(token)
	c.Assert(err, jc.ErrorIsNil)

	s.clock.EXPECT().Now().AnyTimes().Return(now)

	parsedToken, err := jwt.Parse(rawToken, jwt.WithKey(authn.jwtAlg, authn.sharedSecret), jwt.WithClock(s.clock))
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(parsedToken.Subject(), gc.Equals, tokenSubject)
	c.Assert(parsedToken.PrivateClaims()[tunnelIDClaimKey], gc.Equals, tunnelID)
	c.Assert(parsedToken.Issuer(), gc.Equals, tokenIssuer)
	c.Assert(parsedToken.Expiration().Sub(parsedToken.IssuedAt()), gc.Equals, maxTimeout)
}

func (s *authenticationSuite) TestValidatePasswordInvalidToken(c *gc.C) {
	defer s.setupMocks(c).Finish()

	authn := s.newAuthn(c)

	_, err := authn.validatePassword("invalid-token")
	c.Assert(err, gc.ErrorMatches, "failed to decode token: .*")
}

func (s *authenticationSuite) TestValidatePasswordExpiredToken(c *gc.C) {
	defer s.setupMocks(c).Finish()

	authn := s.newAuthn(c)

	now := time.Now()
	deadline := now.Add(maxTimeout)

	tunnelID := "test-tunnel-id"
	token, err := authn.generatePassword(tunnelID, now, deadline)
	c.Assert(err, jc.ErrorIsNil)

	expiry := now.Add(maxTimeout)
	s.clock.EXPECT().Now().AnyTimes().Return(expiry)

	_, err = authn.validatePassword(token)
	c.Assert(err, gc.ErrorMatches, `failed to parse token: "exp" not satisfied`)
}
