// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	"context"
	"errors"
	"testing"

	"github.com/gliderlabs/ssh"
	"github.com/juju/tc"
	sshtesting "github.com/juju/utils/v4/ssh/testing"
	"github.com/lestrrat-go/jwx/v2/jwt"
	gossh "golang.org/x/crypto/ssh"

	coressh "github.com/juju/juju/core/ssh"
	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/pki/test"
)

// TODO(Kian): Remove these once authz and authn are wired into the SSH server.
var _ Authenticator = authenticator{}
var _ Authorizer = authorizer{}

type authenticationSuite struct{}

func TestAuthenticationSuite(t *testing.T) {
	tc.Run(t, &authenticationSuite{})
}

func (s *authenticationSuite) TestPasswordAuthenticationRejectsUnexpectedUser(c *tc.C) {
	ctx := &stubAuthenticationContext{user: "alice", values: map[any]any{}}
	auth := authenticator{
		logger: loggertesting.WrapCheckLog(c),
	}
	c.Check(auth.PasswordAuthentication(ctx, "not-a-token"), tc.IsFalse)
	c.Check(ctx.values[authenticatedViaPublicKey{}], tc.Equals, false)
}

func (s *authenticationSuite) TestPasswordAuthenticationAcceptsJIMMJWT(c *tc.C) {
	token, err := jwt.NewBuilder().Subject("alice").Build()
	c.Assert(err, tc.ErrorIsNil)
	ctx := &stubAuthenticationContext{user: jimmUser, values: map[any]any{}}
	parser := &stubJWTParser{token: token}

	auth := authenticator{jwtParser: parser}
	c.Check(auth.PasswordAuthentication(ctx, "encoded-jwt"), tc.IsTrue)
	c.Check(ctx.values[authenticatedViaPublicKey{}], tc.Equals, false)
	c.Check(ctx.values[userJWT{}], tc.Equals, token)
	c.Check(parser.password, tc.Equals, "encoded-jwt")
}

func (s *authenticationSuite) TestPasswordAuthenticationRejectsInvalidJIMMJWT(c *tc.C) {
	ctx := &stubAuthenticationContext{user: jimmUser, values: map[any]any{}}
	parser := &stubJWTParser{err: errors.New("invalid token")}

	auth := authenticator{
		logger:    loggertesting.WrapCheckLog(c),
		jwtParser: parser,
	}
	c.Check(auth.PasswordAuthentication(ctx, "invalid-jwt"), tc.IsFalse)
	c.Check(ctx.values[authenticatedViaPublicKey{}], tc.Equals, false)
	c.Check(ctx.values[userJWT{}], tc.IsNil)
	c.Check(parser.password, tc.Equals, "invalid-jwt")
}

func (s *authenticationSuite) TestPasswordAuthenticationAcceptsReverseTunnel(c *tc.C) {
	ctx := &stubAuthenticationContext{user: coressh.ReverseTunnelUser, values: map[any]any{}}
	tunnelTracker := &stubTunnelAuthenticator{tunnelID: "tunnel-uuid"}

	auth := authenticator{tunnelTracker: tunnelTracker}
	c.Check(auth.PasswordAuthentication(ctx, "tunnel-password"), tc.IsTrue)
	c.Check(ctx.values[authenticatedViaPublicKey{}], tc.Equals, false)
	c.Check(ctx.values[tunnelIDKey{}], tc.Equals, "tunnel-uuid")
	c.Check(tunnelTracker.username, tc.Equals, coressh.ReverseTunnelUser)
	c.Check(tunnelTracker.password, tc.Equals, "tunnel-password")
}

func (s *authenticationSuite) TestPasswordAuthenticationRejectsInvalidReverseTunnel(c *tc.C) {
	ctx := &stubAuthenticationContext{user: coressh.ReverseTunnelUser, values: map[any]any{}}
	tunnelTracker := &stubTunnelAuthenticator{err: errors.New("invalid credentials")}

	auth := authenticator{
		logger:        loggertesting.WrapCheckLog(c),
		tunnelTracker: tunnelTracker,
	}
	c.Check(auth.PasswordAuthentication(ctx, "invalid-password"), tc.IsFalse)
	c.Check(ctx.values[authenticatedViaPublicKey{}], tc.Equals, false)
	c.Check(ctx.values[tunnelIDKey{}], tc.IsNil)
	c.Check(tunnelTracker.username, tc.Equals, coressh.ReverseTunnelUser)
	c.Check(tunnelTracker.password, tc.Equals, "invalid-password")
}

func (s *authenticationSuite) TestPublicKeyAuthenticationAcceptsUsersKey(c *tc.C) {
	signer := newSigner(c)
	ctx := &stubAuthenticationContext{user: "alice", values: map[any]any{}}
	publicKeys := &stubUserPublicKeyService{keys: []gossh.PublicKey{signer.PublicKey()}}

	auth := authenticator{
		publicKeys: publicKeys,
	}
	c.Check(auth.PublicKeyAuthentication(ctx, signer.PublicKey()), tc.IsTrue)
	c.Check(ctx.values[authenticatedViaPublicKey{}], tc.Equals, true)
	c.Check(publicKeys.user, tc.Equals, "alice")
}

func (s *authenticationSuite) TestPublicKeyAuthenticationRejectsUnauthorizedKey(c *tc.C) {
	ctx := &stubAuthenticationContext{user: "alice", values: map[any]any{}}
	unauthorizedKey := parseAuthorizedKey(c, sshtesting.ValidKeyOne.Key)

	auth := authenticator{
		publicKeys: &stubUserPublicKeyService{keys: []gossh.PublicKey{unauthorizedKey}},
	}
	c.Check(auth.PublicKeyAuthentication(ctx, newSigner(c).PublicKey()), tc.IsFalse)
	c.Check(ctx.values[authenticatedViaPublicKey{}], tc.IsNil)
}

func (s *authenticationSuite) TestPublicKeyAuthenticationRejectsKeyLookupError(c *tc.C) {
	ctx := &stubAuthenticationContext{user: "alice", values: map[any]any{}}

	auth := authenticator{
		logger:     loggertesting.WrapCheckLog(c),
		publicKeys: &stubUserPublicKeyService{err: errors.New("boom")},
	}
	c.Check(auth.PublicKeyAuthentication(ctx, newSigner(c).PublicKey()), tc.IsFalse)
	c.Check(ctx.values[authenticatedViaPublicKey{}], tc.IsNil)
}

func newSigner(c *tc.C) gossh.Signer {
	privateKey, err := test.InsecureKeyProfile()
	c.Assert(err, tc.ErrorIsNil)

	signer, err := gossh.NewSignerFromSigner(privateKey)
	c.Assert(err, tc.ErrorIsNil)
	return signer
}

func parseAuthorizedKey(c *tc.C, key string) gossh.PublicKey {
	publicKey, _, _, _, err := gossh.ParseAuthorizedKey([]byte(key))
	c.Assert(err, tc.ErrorIsNil)
	return publicKey
}

type stubJWTParser struct {
	token    jwt.Token
	err      error
	password string
}

func (s *stubJWTParser) Parse(_ context.Context, password string) (jwt.Token, error) {
	s.password = password
	return s.token, s.err
}

type stubTunnelAuthenticator struct {
	tunnelID string
	err      error
	username string
	password string
}

func (s *stubTunnelAuthenticator) AuthenticateTunnel(username, password string) (string, error) {
	s.username = username
	s.password = password
	return s.tunnelID, s.err
}

type stubUserPublicKeyService struct {
	keys []gossh.PublicKey
	err  error
	user string
}

func (s *stubUserPublicKeyService) PublicKeys(_ context.Context, username string) ([]gossh.PublicKey, error) {
	s.user = username
	return s.keys, s.err
}

type stubAuthenticationContext struct {
	ssh.Context
	user   string
	values map[any]any
}

func (c *stubAuthenticationContext) User() string {
	return c.user
}

func (c *stubAuthenticationContext) SetValue(key, value any) {
	c.values[key] = value
}

func (c *stubAuthenticationContext) Value(key any) any {
	return c.values[key]
}
