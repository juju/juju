// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package auth

import (
	"context"
	"testing"

	"github.com/juju/tc"
)

// auditContextSuite represents a set of tests asserting the contracts and
// behavior of the available authentication audit context funcs on offer by this
// package.
type auditContextSuite struct{}

// TestAuditContextSuite runs the tests contained within [auditContextSuite].
func TestAuditContextSuite(t *testing.T) {
	tc.Run(t, auditContextSuite{})
}

// TestAuditActorTypeValueNotSet checks that when getting the audit actor type
// from a context and it has not been set results in an empty
// [AuthenticatedActorType] value to the caller.
func (auditContextSuite) TestAuditActorTypeValueNotSet(c *tc.C) {
	ctx := context.Background()
	gotVal := AuditActorTypeValue(ctx)
	c.Check(gotVal, tc.Equals, AuthenticatedActorType(""))
}

// TestAuditActorUUIDValueNotSet checks that when getting the audit actor UUID
// from a context and it has not been set results in an empty string to the
// caller.
func (auditContextSuite) TestAuditActorUUIDValueNotSet(c *tc.C) {
	ctx := context.Background()
	gotVal := AuditActorUUIDValue(ctx)
	c.Check(gotVal, tc.Equals, "")
}

// TestAuditAuthenticatorNameValueNotSet checks that when getting the audit
// authenticator name from a context and it has not been set results in an empty
// string to the caller.
func (auditContextSuite) TestAuditAuthenticatorNameValueNotSet(c *tc.C) {
	ctx := context.Background()
	gotVal := AuditAuthenticatorNameValue(ctx)
	c.Check(gotVal, tc.Equals, "")
}

// TestAuditAuthenticatorUsedValueNotSet checks that when getting the audit
// authenticator used from a context and it has not been set results in an empty
// string to the caller.
func (auditContextSuite) TestAuditAuthenticatorUsedValueNotSet(c *tc.C) {
	ctx := context.Background()
	gotVal := AuditAuthenticatorUsedValue(ctx)
	c.Check(gotVal, tc.Equals, "")
}

// TestWithAuditActorType checks that a context is returned with the audit actor
// actor type correctly set.
func (auditContextSuite) TestWithAuditActorType(c *tc.C) {
	ctx := context.Background()
	ctx = WithAuditActorType(ctx, AuthenticatedEntityTypeController)
	gotVal := AuditActorTypeValue(ctx)
	c.Check(gotVal, tc.Equals, AuthenticatedEntityTypeController)
}

// TestWithAuditActorUUID checks that a context is returned with the audit actor
// UUID correctly set.
func (auditContextSuite) TestWithAuditActorUUID(c *tc.C) {
	ctx := context.Background()
	ctx = WithAuditActorUUID(ctx, "1234")
	gotVal := AuditActorUUIDValue(ctx)
	c.Check(gotVal, tc.Equals, "1234")
}

// TestWithAuditAuthenticatorName checks that a context is returned with the
// audit authenticator name correctly set.
func (auditContextSuite) TestWithAuditAuthenticatorName(c *tc.C) {
	ctx := context.Background()
	ctx = WithAuditAuthenticatorName(ctx, "custom-authenticator-inst-1")
	gotVal := AuditAuthenticatorNameValue(ctx)
	c.Check(gotVal, tc.Equals, "custom-authenticator-inst-1")
}

// TestWithAuditAuthenticatorUsed checks that a context is returned with the
// audit authenticator used correctly set.
func (auditContextSuite) TestWithAuditAuthenticatorUsed(c *tc.C) {
	ctx := context.Background()
	ctx = WithAuditAuthenticatorUsed(ctx, "local-controller-user-db")
	gotVal := AuditAuthenticatorUsedValue(ctx)
	c.Check(gotVal, tc.Equals, "local-controller-user-db")
}
