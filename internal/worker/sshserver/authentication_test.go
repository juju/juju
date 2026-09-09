// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	"context"
	"errors"
	"testing"

	"github.com/juju/tc"
	sshtesting "github.com/juju/utils/v4/ssh/testing"
	ssh "github.com/tailscale/gliderssh"
	gossh "golang.org/x/crypto/ssh"

	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/pki/test"
)

type authenticationSuite struct{}

func TestAuthenticationSuite(t *testing.T) {
	tc.Run(t, &authenticationSuite{})
}

func (s *authenticationSuite) TestPasswordAuthenticationRejectsAllPasswords(c *tc.C) {
	// The reverse-tunnel and external-auth password paths moved to the
	// HTTP upgrade endpoints on the API server; no password is valid on
	// the jump server any more.
	ctx := &stubAuthenticationContext{user: "alice", values: map[any]any{}}
	auth := authenticator{
		logger: loggertesting.WrapCheckLog(c),
	}
	authenticated, err := auth.PasswordAuthentication(ctx, "anything")
	c.Check(err, tc.ErrorIsNil)
	c.Check(authenticated, tc.IsFalse)
}

func (s *authenticationSuite) TestPublicKeyAuthenticationAcceptsUsersKey(c *tc.C) {
	signer := newSigner(c)
	ctx := &stubAuthenticationContext{user: "alice", values: map[any]any{}}
	publicKeys := &stubUserPublicKeyService{keys: []gossh.PublicKey{signer.PublicKey()}}

	auth := authenticator{
		publicKeys: publicKeys,
	}
	authenticated, err := auth.PublicKeyAuthentication(ctx, signer.PublicKey())
	c.Check(err, tc.ErrorIsNil)
	c.Check(authenticated, tc.IsTrue)
	c.Check(ctx.values[authenticatedViaPublicKey{}], tc.Equals, true)
	c.Check(publicKeys.user, tc.Equals, "alice")
}

func (s *authenticationSuite) TestPublicKeyAuthenticationRejectsUnauthorizedKey(c *tc.C) {
	ctx := &stubAuthenticationContext{user: "alice", values: map[any]any{}}
	unauthorizedKey := parseAuthorizedKey(c, sshtesting.ValidKeyOne.Key)

	auth := authenticator{
		publicKeys: &stubUserPublicKeyService{keys: []gossh.PublicKey{unauthorizedKey}},
	}
	authenticated, err := auth.PublicKeyAuthentication(ctx, newSigner(c).PublicKey())
	c.Check(err, tc.ErrorIsNil)
	c.Check(authenticated, tc.IsFalse)
	c.Check(ctx.values[authenticatedViaPublicKey{}], tc.IsNil)
}

func (s *authenticationSuite) TestPublicKeyAuthenticationRejectsKeyLookupError(c *tc.C) {
	ctx := &stubAuthenticationContext{user: "alice", values: map[any]any{}}

	auth := authenticator{
		logger:     loggertesting.WrapCheckLog(c),
		publicKeys: &stubUserPublicKeyService{err: errors.New("boom")},
	}
	authenticated, err := auth.PublicKeyAuthentication(ctx, newSigner(c).PublicKey())
	c.Check(err, tc.ErrorMatches, "getting SSH public keys for user \\\"alice\\\": boom")
	c.Check(authenticated, tc.IsFalse)
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
