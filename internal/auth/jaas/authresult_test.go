// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package jaas

import (
	"context"
	"testing"

	"github.com/juju/tc"
	"go.uber.org/mock/gomock"

	coreuser "github.com/juju/juju/core/user"
	"github.com/juju/juju/internal/auth"
)

// authResultSuite contains a set of tests for asserting the interface and
// contract of [AuthResult].
type authResultSuite struct {
	userService *MockUserService
}

// TestAuthResultSuite runs all of the tests contained within [authResultSuite].
func TestAuthResultSuite(t *testing.T) {
	tc.Run(t, &authResultSuite{})
}

// SetupMocks sets up the mocks for the [authResultSuite].
func (s *authResultSuite) setupMocks(c *tc.C) *gomock.Controller {
	ctrl := gomock.NewController(c)
	s.userService = NewMockUserService(ctrl)

	c.Cleanup(func() {
		s.userService = nil
	})
	return ctrl
}

// TestEnsuresExternalUserWithDefaultDomain ensures that the authenticated actor
// is created in the user service and when no domain has been set by JAAS a
// default domain is used.
func (s *authResultSuite) TestEnsuresExternalUserWithDefaultDomain(c *tc.C) {
	defer s.setupMocks(c).Finish()

	userName := tc.Must2(c, coreuser.ParseNameWithDomain, "bob", "jaas")
	userUUID := tc.Must(c, coreuser.NewUUID)

	s.userService.EXPECT().EnsureExternalUser(
		gomock.Any(),
		userName,
		"bob",
	).Return(userUUID, nil)

	authRes := AuthResult{
		jaasIdentifier: "jaas-1",
		userDomain:     "", // left blank to make sure the default gets set
		userName:       "bob",
		userService:    s.userService,
	}

	actorType, actorUUID, err := authRes.AuthenticatedActor(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(actorType, tc.Equals, auth.AuthenticatedEntityTypeUser)
	c.Check(actorUUID, tc.Equals, userUUID.String())
}

// TestEnsureExternalUserWithDomain ensures that the authenticated actor is
// created maintaining the domain that has been passed down from JAAS.
func (s *authResultSuite) TestEnsureExternalUserWithDomain(c *tc.C) {
	defer s.setupMocks(c).Finish()

	userName := tc.Must2(c, coreuser.ParseNameWithDomain, "bob", "juju.is")
	userUUID := tc.Must(c, coreuser.NewUUID)

	s.userService.EXPECT().EnsureExternalUser(
		gomock.Any(),
		userName,
		"bob",
	).Return(userUUID, nil)

	authRes := AuthResult{
		jaasIdentifier: "jaas-1",
		userDomain:     "juju.is",
		userName:       "bob",
		userService:    s.userService,
	}

	actorType, actorUUID, err := authRes.AuthenticatedActor(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(actorType, tc.Equals, auth.AuthenticatedEntityTypeUser)
	c.Check(actorUUID, tc.Equals, userUUID.String())
}

// TestWithAuditContext ensures that the [AuthResult] correctly sets the
// required audit information on the returned context.
func (s *authResultSuite) TestWithAuditContext(c *tc.C) {
	defer s.setupMocks(c).Finish()

	userName := tc.Must2(c, coreuser.ParseNameWithDomain, "bob", "jaas")
	userUUID := tc.Must(c, coreuser.NewUUID)

	s.userService.EXPECT().EnsureExternalUser(
		gomock.Any(),
		userName,
		"bob",
	).Return(userUUID, nil)

	authRes := AuthResult{
		jaasIdentifier: "jaas-1",
		userDomain:     "",
		userName:       "bob",
		userService:    s.userService,
	}

	ctx := context.Background()
	ctx, err := authRes.WithAuditContext(ctx)
	c.Check(err, tc.ErrorIsNil)
	c.Check(auth.AuditActorTypeValue(ctx), tc.Equals, auth.AuthenticatedEntityTypeUser)
	c.Check(auth.AuditActorUUIDValue(ctx), tc.Equals, userUUID.String())
	c.Check(auth.AuditAuthenticatorNameValue(ctx), tc.Equals, "jaas-1")
	c.Check(auth.AuditAuthenticatorUsedValue(ctx), tc.Equals, "jaas")
}
