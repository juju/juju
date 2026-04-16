// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package localuser

import (
	"context"
	"testing"

	"github.com/juju/tc"

	coremodel "github.com/juju/juju/core/model"
	coreuser "github.com/juju/juju/core/user"
	"github.com/juju/juju/internal/auth"
)

// authResultSuite is a set of tests for asserting the contracts and behaviours
// of [AuthResult].
type authResultSuite struct{}

// TestAuthResultSuite runs all of the tests contained within [authResultSuite].
func TestAuthResultSuite(t *testing.T) {
	tc.Run(t, authResultSuite{})
}

// TestAuthenticatedActor asserts that the users uuid and actor type are
// correctly returned from [AuthResult].
func (authResultSuite) TestAuthenticatedActor(c *tc.C) {
	modelUUID := tc.Must(c, coremodel.NewUUID)
	userUUID := tc.Must(c, coreuser.NewUUID)

	authResult := AuthResult{
		modelUUID: modelUUID,
		userUUID:  userUUID,
	}

	actorType, gotUUID, err := authResult.AuthenticatedActor(c.Context())
	c.Check(err, tc.ErrorIsNil)
	c.Check(actorType, tc.Equals, auth.AuthenticatedEntityTypeUser)
	c.Check(gotUUID, tc.Equals, userUUID.String())
}

// TestWithAuditContext ensures that the [AuthResult] correctly sets the
// required audit information on the returned context.
func (authResultSuite) TestWithAuditContext(c *tc.C) {
	modelUUID := tc.Must(c, coremodel.NewUUID)
	userUUID := tc.Must(c, coreuser.NewUUID)

	authResult := AuthResult{
		modelUUID: modelUUID,
		userUUID:  userUUID,
	}

	ctx, err := authResult.WithAuditContext(context.Background())
	c.Check(err, tc.ErrorIsNil)
	c.Check(auth.AuditActorTypeValue(ctx), tc.Equals, auth.AuthenticatedEntityTypeUser)
	c.Check(auth.AuditActorUUIDValue(ctx), tc.Equals, userUUID.String())
	c.Check(auth.AuditAuthenticatorNameValue(ctx), tc.Equals, "model-"+modelUUID.String())
	c.Check(auth.AuditAuthenticatorUsedValue(ctx), tc.Equals, "local")
}
