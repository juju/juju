// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package sshserver

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/base64"
	"encoding/pem"

	"github.com/juju/loggo"
	jc "github.com/juju/testing/checkers"
	"github.com/lestrrat-go/jwx/v2/jwt"
	gomock "go.uber.org/mock/gomock"
	"golang.org/x/crypto/ssh"
	gossh "golang.org/x/crypto/ssh"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/core/virtualhostname"
	"github.com/juju/juju/pki/test"
	"github.com/juju/juju/rpc/params"
)

type authenticationSuite struct {
	ctx                 *MockContext
	jwtParser           *MockJWTParser
	facadeClient        *MockFacadeClient
	tunnelAuthenticator *MockTunnelAuthenticator
}

var _ = gc.Suite(&authenticationSuite{})

func (s *authenticationSuite) SetupMocks(c *gc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.jwtParser = NewMockJWTParser(ctrl)
	s.facadeClient = NewMockFacadeClient(ctrl)
	s.ctx = NewMockContext(ctrl)
	s.tunnelAuthenticator = NewMockTunnelAuthenticator(ctrl)

	return ctrl
}

func (s *authenticationSuite) SetupAuthenticator(c *gc.C) *authenticator {
	return &authenticator{
		logger:        loggo.GetLogger("test"),
		jwtParser:     s.jwtParser,
		facadeClient:  s.facadeClient,
		tunnelTracker: s.tunnelAuthenticator,
	}
}

func (s *authenticationSuite) TestPublicKeyAuthentication(c *gc.C) {
	defer s.SetupMocks(c).Finish()

	authenticator := s.SetupAuthenticator(c)

	s.ctx.EXPECT().SetValue(authenticatedViaPublicKey{}, true)

	ok := authenticator.publicKeyAuthentication(s.ctx, nil)
	c.Assert(ok, gc.Equals, true)
}

func (s *authenticationSuite) TestJWTPasswordAuthentication(c *gc.C) {
	defer s.SetupMocks(c).Finish()

	authenticator := s.SetupAuthenticator(c)

	token := jwt.New()
	s.jwtParser.EXPECT().Parse(gomock.Any(), "password").Return(token, nil)
	s.ctx.EXPECT().SetValue(userJWT{}, token)
	s.ctx.EXPECT().SetValue(authenticatedViaPublicKey{}, false)
	s.ctx.EXPECT().User().Return("jimm")

	ok := authenticator.passwordAuthentication(s.ctx, "password")
	c.Assert(ok, gc.Equals, true)
}

func (s *authenticationSuite) TestTunnelPasswordAuthentication(c *gc.C) {
	defer s.SetupMocks(c).Finish()

	authenticator := s.SetupAuthenticator(c)

	s.tunnelAuthenticator.EXPECT().AuthenticateTunnel("juju-reverse-tunnel", "password").Return("tunnel-id", nil)
	s.ctx.EXPECT().SetValue(tunnelIDKey{}, "tunnel-id")
	s.ctx.EXPECT().SetValue(authenticatedViaPublicKey{}, false)
	s.ctx.EXPECT().User().Return("juju-reverse-tunnel").Times(3)

	ok := authenticator.passwordAuthentication(s.ctx, "password")
	c.Assert(ok, gc.Equals, true)
}

func (s *authenticationSuite) TestPasswordAuthenticationInvalidUser(c *gc.C) {
	defer s.SetupMocks(c).Finish()

	authenticator := s.SetupAuthenticator(c)

	s.ctx.EXPECT().User().Return("fake-user").Times(2)
	s.ctx.EXPECT().SetValue(authenticatedViaPublicKey{}, false)

	ok := authenticator.passwordAuthentication(s.ctx, "password")
	c.Assert(ok, jc.IsFalse)
}

func (s *authenticationSuite) TestGatherModelKeys(c *gc.C) {
	defer s.SetupMocks(c).Finish()

	authenticator := s.SetupAuthenticator(c)

	key, err := test.InsecureKeyProfile()
	c.Assert(err, jc.ErrorIsNil)

	pemKey := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key.(*rsa.PrivateKey)),
	})

	privateKey, err := ssh.ParsePrivateKey(pemKey)
	c.Assert(err, jc.ErrorIsNil)

	publicKeys := []gossh.PublicKey{
		privateKey.PublicKey(),
		privateKey.PublicKey(),
	}

	s.ctx.EXPECT().Value(authenticatedViaPublicKey{}).Return(true)

	s.facadeClient.EXPECT().ListPublicKeysForModel(params.ListAuthorizedKeysArgs{
		ModelUUID: "",
	}).Return(publicKeys, nil)

	finalAuth, err := authenticator.newTerminatingServerAuthenticator(s.ctx, virtualhostname.Info{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(finalAuth.keysToVerify, gc.DeepEquals, publicKeys)
}

func (s *authenticationSuite) TestGatherJWTKey(c *gc.C) {
	defer s.SetupMocks(c).Finish()

	authenticator := s.SetupAuthenticator(c)

	key, err := test.InsecureKeyProfile()
	c.Assert(err, jc.ErrorIsNil)

	pemKey := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key.(*rsa.PrivateKey)),
	})

	privateKey, err := ssh.ParsePrivateKey(pemKey)
	c.Assert(err, jc.ErrorIsNil)

	publicKey := privateKey.PublicKey()

	token, err := jwt.NewBuilder().
		Claim("ssh_public_key", base64.StdEncoding.EncodeToString(publicKey.Marshal())).
		Build()
	c.Assert(err, jc.ErrorIsNil)

	s.ctx.EXPECT().Value(authenticatedViaPublicKey{}).Return(false)
	s.ctx.EXPECT().Value(userJWT{}).Return(token)
	// s.ctx.EXPECT().SetValue(userKeysToVerify{}, []gossh.PublicKey{publicKey})

	finalAuth, err := authenticator.newTerminatingServerAuthenticator(s.ctx, virtualhostname.Info{})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(finalAuth.keysToVerify, gc.DeepEquals, []gossh.PublicKey{publicKey})
}

func (s *authenticationSuite) TestTerminatingServerPublicKeyAuthentication(c *gc.C) {
	defer s.SetupMocks(c).Finish()

	key, err := test.InsecureKeyProfile()
	c.Assert(err, jc.ErrorIsNil)

	pemKey := pem.EncodeToMemory(&pem.Block{
		Type:  "RSA PRIVATE KEY",
		Bytes: x509.MarshalPKCS1PrivateKey(key.(*rsa.PrivateKey)),
	})

	privateKey, err := ssh.ParsePrivateKey(pemKey)
	c.Assert(err, jc.ErrorIsNil)

	publicKey := privateKey.PublicKey()

	finalAuth := terminatingServerAuthenticator{
		keysToVerify: []gossh.PublicKey{publicKey},
	}

	ok := finalAuth.PublicKeyAuthentication(s.ctx, publicKey)
	c.Assert(ok, jc.IsTrue)
}
