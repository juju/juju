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
	gossh "golang.org/x/crypto/ssh"

	loggertesting "github.com/juju/juju/internal/logger/testing"
	"github.com/juju/juju/internal/pki/test"
)

type authenticationSuite struct{}

func TestAuthenticationSuite(t *testing.T) {
	tc.Run(t, &authenticationSuite{})
}

func (s *authenticationSuite) TestPasswordAuthenticationRejectsUnexpectedUser(c *tc.C) {
	ctx := &authenticationContext{user: "alice", values: map[any]any{}}
	auth := authenticator{
		logger: loggertesting.WrapCheckLog(c),
	}
	c.Check(auth.PasswordAuthentication(ctx, "not-a-token"), tc.IsFalse)
	c.Check(ctx.values[authenticatedViaPublicKey{}], tc.Equals, false)
}

func (s *authenticationSuite) TestPublicKeyAuthenticationAcceptsUsersKey(c *tc.C) {
	signer := newSigner(c)
	ctx := &authenticationContext{user: "alice", values: map[any]any{}}
	publicKeys := &stubUserPublicKeyService{keys: []gossh.PublicKey{signer.PublicKey()}}

	auth := authenticator{
		publicKeys: publicKeys,
	}
	c.Check(auth.PublicKeyAuthentication(ctx, signer.PublicKey()), tc.IsTrue)
	c.Check(ctx.values[authenticatedViaPublicKey{}], tc.Equals, true)
	c.Check(publicKeys.user, tc.Equals, "alice")
}

func (s *authenticationSuite) TestPublicKeyAuthenticationRejectsUnregisteredKey(c *tc.C) {
	ctx := &authenticationContext{user: "alice", values: map[any]any{}}
	unregisteredKey := parseAuthorizedKey(c, sshtesting.ValidKeyOne.Key)

	auth := authenticator{
		publicKeys: &stubUserPublicKeyService{keys: []gossh.PublicKey{unregisteredKey}},
	}
	c.Check(auth.PublicKeyAuthentication(ctx, newSigner(c).PublicKey()), tc.IsFalse)
	c.Check(ctx.values[authenticatedViaPublicKey{}], tc.IsNil)
}

func (s *authenticationSuite) TestPublicKeyAuthenticationRejectsKeyLookupError(c *tc.C) {
	ctx := &authenticationContext{user: "alice", values: map[any]any{}}

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

type stubUserPublicKeyService struct {
	keys []gossh.PublicKey
	err  error
	user string
}

func (s *stubUserPublicKeyService) PublicKeys(_ context.Context, username string) ([]gossh.PublicKey, error) {
	s.user = username
	return s.keys, s.err
}

type authenticationContext struct {
	ssh.Context
	user   string
	values map[any]any
}

func (c *authenticationContext) User() string {
	return c.user
}

func (c *authenticationContext) SetValue(key, value any) {
	c.values[key] = value
}

func (c *authenticationContext) Value(key any) any {
	return c.values[key]
}
