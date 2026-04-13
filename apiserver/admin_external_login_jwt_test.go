// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"context"
	"net/http"
	stdtesting "testing"

	"github.com/juju/errors"
	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/api"
	"github.com/juju/juju/api/base"
	apitesting "github.com/juju/juju/apiserver/testing"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	jujuversion "github.com/juju/juju/core/version"
	accesserrors "github.com/juju/juju/domain/access/errors"
	jujutesting "github.com/juju/juju/juju/testing"
	"github.com/juju/juju/rpc/params"
)

// jwtLoginProvider is a LoginProvider that authenticates using a JWT token.
type jwtLoginProvider struct {
	tag   names.Tag
	token string
}

func (p *jwtLoginProvider) Login(ctx context.Context, caller base.APICaller) (*api.LoginResultParams, error) {
	var result params.LoginResult
	err := caller.APICall(ctx, "Admin", 3, "", "Login", &params.LoginRequest{
		AuthTag:       p.tag.String(),
		Token:         p.token,
		ClientVersion: jujuversion.Current.String(),
	}, &result)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return api.NewLoginResultParams(result)
}

func (p *jwtLoginProvider) AuthHeader() (http.Header, error) {
	h := make(http.Header)
	h.Set("Authorization", "Bearer "+p.token)
	return h, nil
}

func (p *jwtLoginProvider) String() string { return "JWTLoginProvider" }

// externalUserJWTLoginSuite tests external user creation via JWT login.
type externalUserJWTLoginSuite struct {
	jujutesting.ApiServerSuite
}

func TestExternalUserJWTLoginSuite(t *stdtesting.T) {
	tc.Run(t, &externalUserJWTLoginSuite{})
}

func (s *externalUserJWTLoginSuite) SetUpTest(c *tc.C) {
	s.WithJWTTokenParser = &apitesting.InsecureJWTParser{}
	s.ApiServerSuite.SetUpTest(c)
}

// TestExternalUserCreatedOnJWTLogin verifies that an external user is
// inserted into Juju's database after successfully authenticating via the
// JWT path (the modern JAAS/JIMM flow).
func (s *externalUserJWTLoginSuite) TestExternalUserCreatedOnJWTLogin(c *tc.C) {
	accessService := s.ControllerDomainServices(c).Access()

	// The everyone@external user must exist as a user record (it's created
	// during bootstrap and serves as the creator of other external users),
	// but for the JWT path we do NOT need to grant it any permissions.
	// This verifies that JWT login works without everyone@external having
	// controller access — unlike the macaroon path which gates on that.
	err := accessService.AddExternalUser(c.Context(), permission.EveryoneUserName, "", s.AdminUserUUID)
	c.Assert(err, tc.ErrorIsNil)

	// Confirm the external user does not yet exist in Juju's database.
	externalUserName := tc.Must1(c, user.NewName, "testuser@external")
	_, err = accessService.GetUserByName(c.Context(), externalUserName)
	c.Assert(err, tc.ErrorIs, accesserrors.UserNotFound,
		tc.Commentf("testuser@external should not exist before login"))

	// Build a JWT token for "user-testuser@external" with superuser access
	// on the controller. The token is base64-encoded, matching the format
	// JIMM sends to Juju controllers.
	controllerTag := names.NewControllerTag(s.ControllerUUID)
	token, err := apitesting.NewEncodedJWT(apitesting.JWTParams{
		Controller: s.ControllerUUID,
		User:       "user-testuser@external",
		Access: map[string]string{
			controllerTag.String(): "superuser",
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	// Open an API connection using the JWT login provider. This triggers:
	//   1. Client sends LoginRequest with Token set (the JWT).
	//   2. Macaroon authenticator returns NotSupported (no macaroons).
	//   3. JWT authenticator parses the token, extracts user-testuser@external.
	//   4. admin.authenticate() validates permissions from the JWT claims.
	//   5. On success, admin.authenticate() calls EnsureExternalUser, inserting
	//      testuser@external into Juju's user table.
	info := s.ControllerModelApiInfo()
	info.Tag = nil
	info.Password = ""
	info.Macaroons = nil
	apiState, err := api.Open(c.Context(), info, api.DialOpts{
		LoginProvider: &jwtLoginProvider{
			tag:   names.NewUserTag("testuser@external"),
			token: token,
		},
	})
	c.Assert(err, tc.ErrorIsNil)
	defer func() { _ = apiState.Close() }()

	// The external user must now exist in Juju's database.
	_, err = accessService.GetUserByName(c.Context(), externalUserName)
	c.Assert(err, tc.ErrorIsNil,
		tc.Commentf("testuser@external was not created in Juju's DB after JWT login"))
}

// TestExternalUserCreatedOnJWTLogin verifies that an external user is
// inserted into Juju's database after successfully authenticating via the
// JWT path (the modern JAAS/JIMM flow).
func (s *externalUserJWTLoginSuite) TestLoginWithAdminUserJWT(c *tc.C) {
	accessService := s.ControllerDomainServices(c).Access()

	// The everyone@external user must exist as a user record (it's created
	// during bootstrap and serves as the creator of other external users),
	// but for the JWT path we do NOT need to grant it any permissions.
	// This verifies that JWT login works without everyone@external having
	// controller access — unlike the macaroon path which gates on that.
	err := accessService.AddExternalUser(c.Context(), permission.EveryoneUserName, "", s.AdminUserUUID)
	c.Assert(err, tc.ErrorIsNil)

	// Confirm the admin user exists in Juju's database.
	userName := tc.Must1(c, user.NewName, "admin")
	_, err = accessService.GetUserByName(c.Context(), userName)
	c.Assert(err, tc.ErrorIsNil, tc.Commentf("admin user should exist before login"))

	// Build a JWT token for "user-testuser@external" with superuser access
	// on the controller. The token is base64-encoded, matching the format
	// JIMM sends to Juju controllers.
	controllerTag := names.NewControllerTag(s.ControllerUUID)
	token, err := apitesting.NewEncodedJWT(apitesting.JWTParams{
		Controller: s.ControllerUUID,
		User:       "user-admin",
		Access: map[string]string{
			controllerTag.String(): "superuser",
		},
	})
	c.Assert(err, tc.ErrorIsNil)

	// Open an API connection using the JWT login provider. This triggers:
	//   1. Client sends LoginRequest with Token set (the JWT).
	//   2. Macaroon authenticator returns NotSupported (no macaroons).
	//   3. JWT authenticator parses the token, extracts user-testuser@external.
	//   4. admin.authenticate() validates permissions from the JWT claims.
	//   5. On success, admin.authenticate() calls EnsureExternalUser, inserting
	//      testuser@external into Juju's user table.
	info := s.ControllerModelApiInfo()
	info.Tag = nil
	info.Password = ""
	info.Macaroons = nil
	apiState, err := api.Open(c.Context(), info, api.DialOpts{
		LoginProvider: &jwtLoginProvider{
			tag:   names.NewUserTag("admin"),
			token: token,
		},
	})
	// Login should succeed with the admin user.
	c.Assert(err, tc.ErrorIsNil)
	defer func() { _ = apiState.Close() }()
}

// TestExternalUserNotCreatedWhenJWTLoginUnauthorized verifies that an external
// user is NOT inserted into Juju's database when the JWT token carries no
// access permissions. The login should fail and no user row should be created.
func (s *externalUserJWTLoginSuite) TestExternalUserNotCreatedWhenJWTLoginUnauthorized(c *tc.C) {
	accessService := s.ControllerDomainServices(c).Access()

	err := accessService.AddExternalUser(c.Context(), permission.EveryoneUserName, "", s.AdminUserUUID)
	c.Assert(err, tc.ErrorIsNil)

	externalUserName := tc.Must1(c, user.NewName, "testuser@external")
	_, err = accessService.GetUserByName(c.Context(), externalUserName)
	c.Assert(err, tc.ErrorIs, accesserrors.UserNotFound)

	// Build a JWT with no access claims — the user is authenticated but
	// has zero permissions on the controller.
	token, err := apitesting.NewEncodedJWT(apitesting.JWTParams{
		Controller: s.ControllerUUID,
		User:       "user-testuser@external",
		Access:     map[string]string{},
	})
	c.Assert(err, tc.ErrorIsNil)

	info := s.ControllerModelApiInfo()
	info.Tag = nil
	info.Password = ""
	info.Macaroons = nil
	_, err = api.Open(c.Context(), info, api.DialOpts{
		LoginProvider: &jwtLoginProvider{
			tag:   names.NewUserTag("testuser@external"),
			token: token,
		},
	})
	c.Assert(err, tc.NotNil)

	// Failed login must not create an external user row.
	_, err = accessService.GetUserByName(c.Context(), externalUserName)
	c.Assert(err, tc.ErrorIs, accesserrors.UserNotFound,
		tc.Commentf("testuser@external should not be created when JWT login is unauthorised"))
}
