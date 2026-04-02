// Copyright 2026 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package apiserver_test

import (
	"context"
	stdtesting "testing"

	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakery/checkers"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/bakerytest"
	"github.com/go-macaroon-bakery/macaroon-bakery/v3/httpbakery"
	"github.com/juju/names/v6"
	"github.com/juju/tc"

	"github.com/juju/juju/api"
	jujucontroller "github.com/juju/juju/controller"
	"github.com/juju/juju/core/permission"
	"github.com/juju/juju/core/user"
	"github.com/juju/juju/domain/access"
	accesserrors "github.com/juju/juju/domain/access/errors"
	jujutesting "github.com/juju/juju/juju/testing"
)

type externalUserLoginSuite struct {
	jujutesting.ApiServerSuite
	discharger *bakerytest.Discharger
}

func TestExternalUserLoginSuite(t *stdtesting.T) {
	tc.Run(t, &externalUserLoginSuite{})
}

func (s *externalUserLoginSuite) SetUpTest(c *tc.C) {
	s.discharger = bakerytest.NewDischarger(nil)
	// Configure the discharger to identify any login attempt as "testuser".
	// This simulates a JAAS/JIMM identity provider declaring the username.
	s.discharger.CheckerP = httpbakery.ThirdPartyCaveatCheckerPFunc(
		func(_ context.Context, _ httpbakery.ThirdPartyCaveatCheckerParams) ([]checkers.Caveat, error) {
			return []checkers.Caveat{
				checkers.DeclaredCaveat("username", "testuser"),
			}, nil
		},
	)
	// Set the identity URL so that admin.authenticate() uses external macaroon auth.
	s.ControllerConfigAttrs = map[string]interface{}{
		jujucontroller.IdentityURL: s.discharger.Location(),
	}
	s.ApiServerSuite.SetUpTest(c)
}

func (s *externalUserLoginSuite) TearDownTest(c *tc.C) {
	s.ApiServerSuite.TearDownTest(c)
	if s.discharger != nil {
		s.discharger.Close()
		s.discharger = nil
	}
}

// TestExternalUserCreatedOnMacaroonLogin verifies that an external user is
// inserted into Juju's database after successfully authenticating via the
// external macaroon path.
//
// The test uses an httpbakery discharger configured (in SetUpTest) to declare
// "username=testuser" for every discharge request. When api.Open is called
// with no credentials but with a BakeryClient:
//  1. The server returns DischargeRequired with a macaroon pointing to the
//     identity URL (s.discharger).
//  2. The httpbakery client discharges the macaroon; the discharger
//     declares "username=testuser".
//  3. The client retries login with the discharged macaroon.
//  4. The macaroon authenticator extracts "testuser" from the declared
//     caveat and produces a "user-testuser@external" tag.
//  5. admin.authenticate() validates permissions, including inherited
//     everyone@external access for first-time external users.
//  6. After successful authorisation, admin.authenticate() persists the
//     external user via EnsureExternalUser.
func (s *externalUserLoginSuite) TestExternalUserCreatedOnMacaroonLogin(c *tc.C) {
	accessService := s.ControllerDomainServices(c).Access()

	// The everyone@external user must exist as a user record (it's created
	// during bootstrap and serves as the creator of other external users).
	err := accessService.AddExternalUser(c.Context(), permission.EveryoneUserName, "", s.AdminUserUUID)
	c.Assert(err, tc.ErrorIsNil)

	// Grant everyone@external login access on the controller.
	// This is needed so that ReadUserAccessLevelForTarget inherits
	// controller access for testuser@external (external users inherit
	// the higher of their own and everyone@external's permissions).
	err = accessService.UpdatePermission(c.Context(), access.UpdatePermissionArgs{
		Subject: permission.EveryoneUserName,
		Change:  permission.Grant,
		AccessSpec: permission.AccessSpec{
			Target: permission.ID{
				ObjectType: permission.Controller,
				Key:        s.ControllerUUID,
			},
			Access: permission.LoginAccess,
		},
	})
	c.Assert(err, tc.IsNil)

	// Confirm the external user does not yet exist in Juju's database.
	externalUserName := tc.Must1(c, user.NewName, "testuser@external")
	_, err = accessService.GetUserByName(c.Context(), externalUserName)
	c.Assert(err, tc.ErrorIs, accesserrors.UserNotFound,
		tc.Commentf("testuser@external should not exist before login"))

	// Open an API connection with no credentials, triggering the external
	// macaroon login flow described in the test doc comment.
	// Use a controller-only login (no ModelTag) because the test only
	// grants everyone@external login access on the controller, not on any
	// model. Controller-only login only checks controller-level permissions.
	info := s.ControllerModelApiInfo()
	info.ModelTag = names.ModelTag{}
	info.Tag = nil
	info.Password = ""
	info.Macaroons = nil
	apiState, err := api.Open(c.Context(), info, api.DialOpts{
		BakeryClient: httpbakery.NewClient(),
	})
	c.Assert(err, tc.ErrorIsNil)
	defer func() { _ = apiState.Close() }()

	// The external user must now exist in Juju's database.
	_, err = accessService.GetUserByName(c.Context(), externalUserName)
	c.Assert(err, tc.ErrorIsNil,
		tc.Commentf("testuser@external was not created in Juju's DB after external macaroon login"))
}

func (s *externalUserLoginSuite) TestExternalUserNotCreatedWhenMacaroonLoginUnauthorized(c *tc.C) {
	accessService := s.ControllerDomainServices(c).Access()

	// everyone@external must exist as a user record, but do not grant it
	// permissions. This keeps external login unauthorised.
	err := accessService.AddExternalUser(c.Context(), permission.EveryoneUserName, "", s.AdminUserUUID)
	c.Assert(err, tc.ErrorIsNil)

	externalUserName := tc.Must1(c, user.NewName, "testuser@external")
	_, err = accessService.GetUserByName(c.Context(), externalUserName)
	c.Assert(err, tc.ErrorIs, accesserrors.UserNotFound)

	info := s.ControllerModelApiInfo()
	info.ModelTag = names.ModelTag{}
	info.Tag = nil
	info.Password = ""
	info.Macaroons = nil
	_, err = api.Open(c.Context(), info, api.DialOpts{
		BakeryClient: httpbakery.NewClient(),
	})
	c.Assert(err, tc.NotNil)

	// Failed login must not create an external user row.
	_, err = accessService.GetUserByName(c.Context(), externalUserName)
	c.Assert(err, tc.ErrorIs, accesserrors.UserNotFound)
}
