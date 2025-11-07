// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jwt_test

import (
	"context"
	"encoding/base64"
	"net/http"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	jc "github.com/juju/testing/checkers"
	gc "gopkg.in/check.v1"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/authentication/jwt"
	"github.com/juju/juju/apiserver/common"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/testing"
)

type loginTokenSuite struct{}

var _ = gc.Suite(&loginTokenSuite{})

func (s *loginTokenSuite) TestAuthenticate(c *gc.C) {
	modelTag := names.NewModelTag("test")
	applicationOfferTag := names.NewApplicationOfferTag("f47ac10b-58cc-4372-a567-0e02b2c3d479")
	tok, err := EncodedJWT(JWTParams{
		Controller: testing.ControllerTag.Id(),
		User:       "user-fred",
		Access: map[string]string{
			testing.ControllerTag.String(): "login",
			modelTag.String():              "write",
			applicationOfferTag.String():   "consume",
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	params := authentication.AuthParams{
		Token: base64.StdEncoding.EncodeToString(tok),
	}

	authenticator := jwt.NewAuthenticator(&testJWTParser{})

	req, err := http.NewRequest("", "", nil)
	c.Assert(err, jc.ErrorIsNil)
	req.Header.Add("Authorization", "Bearer "+params.Token)
	authInfo, err := authenticator.Authenticate(req)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(authInfo.Entity.Tag().String(), gc.Equals, "user-fred")
	perm, err := authInfo.SubjectPermissions(modelTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(perm, gc.Equals, permission.WriteAccess)

	perm, err = authInfo.SubjectPermissions(testing.ControllerTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(perm, gc.Equals, permission.LoginAccess)

	perm, err = authInfo.SubjectPermissions(applicationOfferTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(perm, gc.Equals, permission.ConsumeAccess)
}

func (s *loginTokenSuite) TestAuthenticateInvalidHeader(c *gc.C) {
	authenticator := jwt.NewAuthenticator(&testJWTParser{})
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
	modelTag := names.NewModelTag("test")
	applicationOfferTag := names.NewApplicationOfferTag("f47ac10b-58cc-4372-a567-0e02b2c3d479")
	tok, err := EncodedJWT(JWTParams{
		Controller: testing.ControllerTag.Id(),
		User:       "user-fred",
		Access: map[string]string{
			testing.ControllerTag.String(): "login",
			modelTag.String():              "write",
			applicationOfferTag.String():   "consume",
		},
	})
	c.Assert(err, jc.ErrorIsNil)

	params := authentication.AuthParams{
		Token: base64.StdEncoding.EncodeToString(tok),
	}

	authenticator := jwt.NewAuthenticator(&testJWTParser{})

	authInfo, err := authenticator.AuthenticateLoginRequest(context.Background(), "", "", params)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(authInfo.Entity.Tag().String(), gc.Equals, "user-fred")
	perm, err := authInfo.SubjectPermissions(modelTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(perm, gc.Equals, permission.WriteAccess)

	perm, err = authInfo.SubjectPermissions(testing.ControllerTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(perm, gc.Equals, permission.LoginAccess)

	perm, err = authInfo.SubjectPermissions(applicationOfferTag)
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
	})
	c.Assert(err, jc.ErrorIsNil)

	params := authentication.AuthParams{
		Token: base64.StdEncoding.EncodeToString(tok),
	}

	authenticator := jwt.NewAuthenticator(&testJWTParser{})

	authInfo, err := authenticator.AuthenticateLoginRequest(context.Background(), "", "", params)
	c.Assert(err, jc.ErrorIsNil)

	badUser := jwt.TokenEntity{
		User: names.NewUserTag("wallyworld"),
	}
	perm, err := authInfo.Delegator.SubjectPermissions(badUser, modelTag)
	c.Assert(errors.Is(err, apiservererrors.ErrPerm), jc.IsTrue)
	c.Assert(errors.Is(err, authentication.ErrorEntityMissingPermission), jc.IsTrue)
	c.Assert(perm, gc.Equals, permission.NoAccess)

	badUser = jwt.TokenEntity{
		User: names.NewUserTag(common.EveryoneTagName),
	}
	perm, err = authInfo.Delegator.SubjectPermissions(badUser, modelTag)
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
	})
	c.Assert(err, jc.ErrorIsNil)

	params := authentication.AuthParams{
		Token: base64.StdEncoding.EncodeToString(tok),
	}

	authenticator := jwt.NewAuthenticator(&testJWTParser{})

	authInfo, err := authenticator.AuthenticateLoginRequest(context.Background(), "", "", params)
	c.Assert(err, jc.ErrorIsNil)

	c.Assert(authInfo.Entity.Tag().String(), gc.Equals, "user-fred")

	perm, err := authInfo.SubjectPermissions(testing.ControllerTag)
	c.Assert(err, jc.ErrorIsNil)
	c.Assert(perm, gc.Equals, permission.SuperuserAccess)
}

func (s *loginTokenSuite) TestNotAvailableJWTParser(c *gc.C) {
	authenticator := jwt.NewAuthenticator(&testJWTParser{notReady: true})

	params := authentication.AuthParams{Token: "token"}
	_, err := authenticator.AuthenticateLoginRequest(context.Background(), "", "", params)
	c.Assert(err, jc.ErrorIs, errors.NotImplemented)

	req, err := http.NewRequest("", "", nil)
	c.Assert(err, jc.ErrorIsNil)
	req.Header.Add("Authorization", "Bearer aaaaa")
	_, err = authenticator.Authenticate(req)
	c.Assert(err, jc.ErrorIs, errors.NotImplemented)
}
