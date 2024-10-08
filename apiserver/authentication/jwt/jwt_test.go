// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jwt_test

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"

	"github.com/juju/errors"
	"github.com/juju/names/v5"
	jc "github.com/juju/testing/checkers"
	"github.com/lestrrat-go/jwx/v2/jwk"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/authentication/jwt"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	apitesting "github.com/juju/juju/apiserver/testing"
	coremodel "github.com/juju/juju/core/model"
	modeltesting "github.com/juju/juju/core/model/testing"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/internal/testing"
)

type loginTokenSuite struct {
	url        string
	keySet     jwk.Set
	signingKey jwk.Key
	srv        *httptest.Server
}

var _ = gc.Suite(&loginTokenSuite{})

func (s *loginTokenSuite) SetUpTest(c *gc.C) {
	keySet, signingKey, err := apitesting.NewJWKSet()
	c.Assert(err, jc.ErrorIsNil)
	s.keySet = keySet
	s.signingKey = signingKey

	s.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.RequestURI != "/.well-known/jwks.json" {
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		hdrs := w.Header()
		hdrs.Set(`Content-Type`, `application/json`)
		pub, _ := s.keySet.Key(0)
		_ = json.NewEncoder(w).Encode(pub)
	}))

	s.url = s.srv.URL + "/.well-known/jwks.json"
}

func (s *loginTokenSuite) TearDownTest(_ *gc.C) {
	s.srv.Close()
}

func (s *loginTokenSuite) TestCacheRegistration(c *gc.C) {
	authenticator := jwt.NewAuthenticator(s.url)
	err := authenticator.RegisterJWKSCache(context.Background())
	c.Assert(err, jc.ErrorIsNil)
}

func (s *loginTokenSuite) TestCacheRegistrationFailureWithBadURL(c *gc.C) {
	authenticator := jwt.NewAuthenticator("noexisturl")
	err := authenticator.RegisterJWKSCache(context.Background())
	// We want to make sure that we get an error for a bad url.
	c.Assert(err, gc.NotNil)
}

func (s *loginTokenSuite) TestAuthenticateLoginRequestNotSupported(c *gc.C) {
	authenticator := jwt.NewAuthenticator(s.url)
	_, err := authenticator.AuthenticateLoginRequest(context.Background(), "", "", authentication.AuthParams{Token: ""})
	c.Assert(err, jc.ErrorIs, errors.NotSupported)
}
func (s *loginTokenSuite) TestAuthenticate(c *gc.C) {
	modelUUID := modeltesting.GenModelUUID(c)
	modelTag := names.NewModelTag(modelUUID.String())
	applicationOfferTag := names.NewApplicationOfferTag("f47ac10b-58cc-4372-a567-0e02b2c3d479")
	tok, err := EncodedJWT(JWTParams{
		Controller: testing.ControllerTag.Id(),
		User:       "user-fred",
		Access: map[string]string{
			testing.ControllerTag.String(): "login",
			modelTag.String():              "write",
			applicationOfferTag.String():   "consume",
		},
	}, s.keySet, s.signingKey)
	c.Assert(err, jc.ErrorIsNil)

	params := authentication.AuthParams{
		Token: base64.StdEncoding.EncodeToString(tok),
	}

	authenticator := jwt.NewAuthenticator(s.url)
	err = authenticator.RegisterJWKSCache(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	req, err := http.NewRequest("", "", nil)
	c.Assert(err, jc.ErrorIsNil)
	req.Header.Add("Authorization", "Bearer "+params.Token)
	authInfo, err := authenticator.Authenticate(req)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(authInfo.Entity.Tag().String(), gc.Equals, "user-fred")
	perm, err := authInfo.SubjectPermissions(context.Background(), permission.ID{
		ObjectType: permission.Model,
		Key:        modelTag.Id(),
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(perm, gc.Equals, permission.WriteAccess)

	perm, err = authInfo.SubjectPermissions(context.Background(), permission.ID{
		ObjectType: permission.Controller,
		Key:        testing.ControllerTag.Id(),
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(perm, gc.Equals, permission.LoginAccess)

	perm, err = authInfo.SubjectPermissions(context.Background(), permission.ID{
		ObjectType: permission.Offer,
		Key:        applicationOfferTag.Id(),
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(perm, gc.Equals, permission.ConsumeAccess)
}

func (s *loginTokenSuite) TestAuthenticateInvalidHeader(c *gc.C) {
	authenticator := jwt.NewAuthenticator(s.url)
	req, err := http.NewRequest("", "", nil)
	c.Assert(err, jc.ErrorIsNil)
	_, err = authenticator.Authenticate(req)
	c.Assert(err, gc.ErrorMatches, ".*authorization header missing.*")

	req, err = http.NewRequest("", "", nil)
	c.Assert(err, jc.ErrorIsNil)
	req.Header.Add("Authorization", "Bad Format aaaaa")
	_, err = authenticator.Authenticate(req)
	c.Assert(err, gc.ErrorMatches, ".*authorization header format.*")

	req, err = http.NewRequest("", "", nil)
	c.Assert(err, jc.ErrorIsNil)
	req.Header.Add("Authorization", "Bearer aaaaa")
	_, err = authenticator.Authenticate(req)
	c.Assert(err, gc.ErrorMatches, ".*parsing jwt.*")
}

func (s *loginTokenSuite) TestUsesLoginToken(c *gc.C) {
	uuid, err := coremodel.NewUUID()
	c.Assert(err, jc.ErrorIsNil)
	modelTag := names.NewModelTag(uuid.String())
	applicationOfferTag := names.NewApplicationOfferTag("f47ac10b-58cc-4372-a567-0e02b2c3d479")
	tok, err := EncodedJWT(JWTParams{
		Controller: testing.ControllerTag.Id(),
		User:       "user-fred",
		Access: map[string]string{
			testing.ControllerTag.String(): "login",
			modelTag.String():              "write",
			applicationOfferTag.String():   "consume",
		},
	}, s.keySet, s.signingKey)
	c.Assert(err, jc.ErrorIsNil)

	params := authentication.AuthParams{
		Token: base64.StdEncoding.EncodeToString(tok),
	}

	authenticator := jwt.NewAuthenticator(s.url)
	err = authenticator.RegisterJWKSCache(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	authInfo, err := authenticator.AuthenticateLoginRequest(context.Background(), "", "", params)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(authInfo.Entity.Tag().String(), gc.Equals, "user-fred")
	perm, err := authInfo.SubjectPermissions(context.Background(), permission.ID{
		ObjectType: permission.Model,
		Key:        modelTag.Id(),
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(perm, gc.Equals, permission.WriteAccess)

	perm, err = authInfo.SubjectPermissions(context.Background(), permission.ID{
		ObjectType: permission.Controller,
		Key:        testing.ControllerTag.Id(),
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(perm, gc.Equals, permission.LoginAccess)

	perm, err = authInfo.SubjectPermissions(context.Background(), permission.ID{
		ObjectType: permission.Offer,
		Key:        applicationOfferTag.Id(),
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(perm, gc.Equals, permission.ConsumeAccess)
}

// TestPermissionsForDifferentEntity is trying to assert that if we use the
// permissions func on the AuthInfo for a different user entity we get an error.
// This proves that there is no chance one users permissions can not be used for
// another. This is a regression test to catch a case that was found in the
// original implementation.
func (s *loginTokenSuite) TestPermissionsForDifferentEntity(c *gc.C) {
	modelTag := names.NewModelTag("test")
	tok, err := EncodedJWT(JWTParams{
		Controller: testing.ControllerTag.Id(),
		User:       "user-fred",
		Access: map[string]string{
			testing.ControllerTag.String(): "login",
			modelTag.String():              "write",
		},
	}, s.keySet, s.signingKey)
	c.Assert(err, jc.ErrorIsNil)

	params := authentication.AuthParams{
		Token: base64.StdEncoding.EncodeToString(tok),
	}

	authenticator := jwt.NewAuthenticator(s.url)
	err = authenticator.RegisterJWKSCache(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	authInfo, err := authenticator.AuthenticateLoginRequest(context.Background(), "", "", params)
	c.Assert(err, jc.ErrorIsNil)

	badUser := jwt.TokenEntity{
		User: names.NewUserTag("wallyworld"),
	}
	perm, err := authInfo.Delegator.SubjectPermissions(context.Background(), badUser.User.Id(), permission.ID{
		ObjectType: permission.Model,
		Key:        modelTag.Id(),
	})
	c.Assert(err, jc.ErrorIs, apiservererrors.ErrPerm)
	c.Assert(err, jc.ErrorIs, authentication.ErrorEntityMissingPermission)
	c.Assert(perm, gc.Equals, permission.NoAccess)

	badUser = jwt.TokenEntity{
		User: names.NewUserTag(permission.EveryoneUserName.Name()),
	}
	perm, err = authInfo.Delegator.SubjectPermissions(context.Background(), badUser.User.Id(), permission.ID{
		ObjectType: permission.Model,
		Key:        modelTag.Id(),
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(perm, gc.Equals, permission.NoAccess)
}

func (s *loginTokenSuite) TestControllerSuperuser(c *gc.C) {
	tok, err := EncodedJWT(JWTParams{
		Controller: testing.ControllerTag.Id(),
		User:       "user-fred",
		Access: map[string]string{
			testing.ControllerTag.String(): "superuser",
		},
	}, s.keySet, s.signingKey)
	c.Assert(err, jc.ErrorIsNil)

	params := authentication.AuthParams{
		Token: base64.StdEncoding.EncodeToString(tok),
	}

	authenticator := jwt.NewAuthenticator(s.url)
	err = authenticator.RegisterJWKSCache(context.Background())
	c.Assert(err, jc.ErrorIsNil)

	authInfo, err := authenticator.AuthenticateLoginRequest(context.Background(), "", "", params)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(authInfo.Entity.Tag().String(), gc.Equals, "user-fred")

	perm, err := authInfo.SubjectPermissions(context.Background(), permission.ID{
		ObjectType: permission.Controller,
		Key:        testing.ControllerTag.Id(),
	})
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(perm, gc.Equals, permission.SuperuserAccess)
}
