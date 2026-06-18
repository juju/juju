// Copyright 2023 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jwt_test

import (
	"encoding/base64"
	"net/http"
	"net/http/httptest"
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/apiserver/authentication"
	"github.com/juju/juju/apiserver/authentication/jwt"
	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/httpcontext"
	coremodel "github.com/juju/juju/core/model"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/internal/testing"
)

type loginTokenSuite struct{}

func TestLoginTokenSuite(t *stdtesting.T) {
	tc.Run(t, &loginTokenSuite{})
}

func (s *loginTokenSuite) TestAuthenticate(c *tc.C) {
	modelUUID := tc.Must0(c, coremodel.NewUUID)
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
	})
	c.Assert(err, tc.ErrorIsNil)

	params := authentication.AuthParams{
		Token: base64.StdEncoding.EncodeToString(tok),
	}

	authenticator := jwt.NewAuthenticator(&testJWTParser{})

	req, err := http.NewRequest("", "", nil)
	c.Assert(err, tc.ErrorIsNil)
	req.Header.Add("Authorization", "Bearer "+params.Token)
	authInfo, err := authenticator.Authenticate(req)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(authInfo.Tag.String(), tc.Equals, "user-fred")
	c.Assert(authInfo.IsExternallyAuthenticated, tc.IsTrue)
	perm, err := authInfo.SubjectPermissions(c.Context(), permission.ID{
		ObjectType: permission.Model,
		Key:        modelTag.Id(),
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(perm, tc.Equals, permission.WriteAccess)

	perm, err = authInfo.SubjectPermissions(c.Context(), permission.ID{
		ObjectType: permission.Controller,
		Key:        testing.ControllerTag.Id(),
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(perm, tc.Equals, permission.LoginAccess)

	perm, err = authInfo.SubjectPermissions(c.Context(), permission.ID{
		ObjectType: permission.Offer,
		Key:        applicationOfferTag.Id(),
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(perm, tc.Equals, permission.ConsumeAccess)
}

func (s *loginTokenSuite) TestAuthenticateSetsModelTagFromRequestContext(c *tc.C) {
	tok, err := EncodedJWT(JWTParams{
		Controller: testing.ControllerTag.Id(),
		User:       "user-fred",
		Access: map[string]string{
			testing.ModelTag.String(): "write",
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	authenticator := jwt.NewAuthenticator(&testJWTParser{})
	req, err := http.NewRequest("", "", nil)
	c.Assert(err, tc.ErrorIsNil)
	req.Header.Add("Authorization", "Bearer "+base64.StdEncoding.EncodeToString(tok))
	req = requestWithModelContext(c, testing.ModelTag.Id(), req)

	authInfo, err := authenticator.Authenticate(req)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(authInfo.ModelTag.Id(), tc.Equals, testing.ModelTag.Id())
}

func (s *loginTokenSuite) TestAuthenticateWithoutModelContextLeavesModelTagEmpty(c *tc.C) {
	tok, err := EncodedJWT(JWTParams{
		Controller: testing.ControllerTag.Id(),
		User:       "user-fred",
		Access: map[string]string{
			testing.ModelTag.String(): "write",
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	authenticator := jwt.NewAuthenticator(&testJWTParser{})
	req, err := http.NewRequest("", "", nil)
	c.Assert(err, tc.ErrorIsNil)
	req.Header.Add("Authorization", "Bearer "+base64.StdEncoding.EncodeToString(tok))

	authInfo, err := authenticator.Authenticate(req)
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(authInfo.ModelTag.Id(), tc.Equals, "")
}

func (s *loginTokenSuite) TestAuthenticateInvalidHeader(c *tc.C) {
	authenticator := jwt.NewAuthenticator(&testJWTParser{})
	req, err := http.NewRequest("", "", nil)
	c.Assert(err, tc.ErrorIsNil)
	_, err = authenticator.Authenticate(req)
	c.Assert(err, tc.ErrorMatches, ".*authorization header missing.*")

	req, err = http.NewRequest("", "", nil)
	c.Assert(err, tc.ErrorIsNil)
	req.Header.Add("Authorization", "Bad Format aaaaa")
	_, err = authenticator.Authenticate(req)
	c.Assert(err, tc.ErrorMatches, ".*authorization header format.*")

	req, err = http.NewRequest("", "", nil)
	c.Assert(err, tc.ErrorIsNil)
	req.Header.Add("Authorization", "Bearer aaaaa")
	_, err = authenticator.Authenticate(req)
	c.Assert(err, tc.ErrorMatches, ".*parsing jwt.*")
}

func (s *loginTokenSuite) TestUsesLoginToken(c *tc.C) {
	uuid, err := coremodel.NewUUID()
	c.Assert(err, tc.ErrorIsNil)
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
	})
	c.Assert(err, tc.ErrorIsNil)

	params := authentication.AuthParams{
		Token: base64.StdEncoding.EncodeToString(tok),
	}

	authenticator := jwt.NewAuthenticator(&testJWTParser{})

	authInfo, err := authenticator.AuthenticateLoginRequest(c.Context(), "", "", params)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(authInfo.Tag.String(), tc.Equals, "user-fred")
	c.Assert(authInfo.ModelTag.Id(), tc.Equals, "")
	c.Assert(authInfo.IsExternallyAuthenticated, tc.IsTrue)
	perm, err := authInfo.SubjectPermissions(c.Context(), permission.ID{
		ObjectType: permission.Model,
		Key:        modelTag.Id(),
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(perm, tc.Equals, permission.WriteAccess)
	c.Assert(authInfo.ModelTag.Id(), tc.Equals, "")

	perm, err = authInfo.SubjectPermissions(c.Context(), permission.ID{
		ObjectType: permission.Controller,
		Key:        testing.ControllerTag.Id(),
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(perm, tc.Equals, permission.LoginAccess)

	perm, err = authInfo.SubjectPermissions(c.Context(), permission.ID{
		ObjectType: permission.Offer,
		Key:        applicationOfferTag.Id(),
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(perm, tc.Equals, permission.ConsumeAccess)
}

// TestPermissionsForDifferentEntity is trying to assert that if we use the
// permissions func on the AuthInfo for a different user entity we get an error.
// This proves that there is no chance one users permissions can not be used for
// another. This is a regression test to catch a case that was found in the
// original implementation.
func (s *loginTokenSuite) TestPermissionsForDifferentEntity(c *tc.C) {
	modelTag := names.NewModelTag("test")
	tok, err := EncodedJWT(JWTParams{
		Controller: testing.ControllerTag.Id(),
		User:       "user-fred",
		Access: map[string]string{
			testing.ControllerTag.String(): "login",
			modelTag.String():              "write",
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	params := authentication.AuthParams{
		Token: base64.StdEncoding.EncodeToString(tok),
	}

	authenticator := jwt.NewAuthenticator(&testJWTParser{})

	authInfo, err := authenticator.AuthenticateLoginRequest(c.Context(), "", "", params)
	c.Assert(err, tc.ErrorIsNil)

	badUser := names.NewUserTag("wallyworld")
	perm, err := authInfo.Delegator.SubjectPermissions(c.Context(), badUser.Id(), permission.ID{
		ObjectType: permission.Model,
		Key:        modelTag.Id(),
	})
	c.Assert(err, tc.ErrorIs, apiservererrors.ErrPerm)
	c.Assert(err, tc.ErrorIs, authentication.ErrorEntityMissingPermission)
	c.Assert(perm, tc.Equals, permission.NoAccess)

	badUser = names.NewUserTag(permission.EveryoneUserName.Name())
	perm, err = authInfo.Delegator.SubjectPermissions(c.Context(), badUser.Id(), permission.ID{
		ObjectType: permission.Model,
		Key:        modelTag.Id(),
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(perm, tc.Equals, permission.NoAccess)
}

func (s *loginTokenSuite) TestControllerSuperuser(c *tc.C) {
	tok, err := EncodedJWT(JWTParams{
		Controller: testing.ControllerTag.Id(),
		User:       "user-fred",
		Access: map[string]string{
			testing.ControllerTag.String(): "superuser",
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	params := authentication.AuthParams{
		Token: base64.StdEncoding.EncodeToString(tok),
	}

	authenticator := jwt.NewAuthenticator(&testJWTParser{})

	authInfo, err := authenticator.AuthenticateLoginRequest(c.Context(), "", "", params)
	c.Assert(err, tc.ErrorIsNil)

	c.Assert(authInfo.Tag.String(), tc.Equals, "user-fred")

	perm, err := authInfo.SubjectPermissions(c.Context(), permission.ID{
		ObjectType: permission.Controller,
		Key:        testing.ControllerTag.Id(),
	})
	c.Assert(err, tc.ErrorIsNil)
	c.Assert(perm, tc.Equals, permission.SuperuserAccess)
}

func (s *loginTokenSuite) TestNotAvailableJWTParser(c *tc.C) {
	authenticator := jwt.NewAuthenticator(&testJWTParser{notReady: true})

	params := authentication.AuthParams{Token: "token"}
	_, err := authenticator.AuthenticateLoginRequest(c.Context(), "", "", params)
	c.Assert(err, tc.ErrorIs, errors.NotImplemented)

	req, err := http.NewRequest("", "", nil)
	c.Assert(err, tc.ErrorIsNil)
	req.Header.Add("Authorization", "Bearer aaaaa")
	_, err = authenticator.Authenticate(req)
	c.Assert(err, tc.ErrorIs, errors.NotImplemented)
}

func requestWithModelContext(c *tc.C, modelUUID string, req *http.Request) *http.Request {
	query := req.URL.Query()
	query.Add("modeluuid", modelUUID)
	req.URL.RawQuery = query.Encode()

	var modelReq *http.Request
	handler := &httpcontext.QueryModelHandler{
		Handler: http.HandlerFunc(func(_ http.ResponseWriter, req *http.Request) {
			modelReq = req
		}),
		Query: "modeluuid",
	}
	handler.ServeHTTP(httptest.NewRecorder(), req)
	c.Assert(modelReq, tc.NotNil)
	return modelReq
}
