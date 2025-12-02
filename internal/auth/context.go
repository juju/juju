// Copyright 2025 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package auth

import "context"

// auditActorType is an internal type used to represent the key used for setting
// the audit actor type on a context.
type auditActorType struct{}

// auditActorUUID is an internal type used to represent the key used for setting
// the audit actor UUID on a context.
type auditActorUUID struct{}

// auditAuthenticatorName is an internal type used to represent the key used for
// setting the canonical name of the authenticator used on a context.
// [auditAuthenticatorName] is different from [auditAuthenticatorUsed] as their
// may be more than one [Authenticator] of the same type available.
type auditAuthenticatorName struct{}

// auditAuthenticatorUsed is an internal type used to represent the key used for
// setting the [Authenticator] used on a context.
type auditAuthenticatorUsed struct{}

// AuditActorTypeValue returns actor type recorded in authentication audit
// information on the context. If no value has been set on the context then a
// zero value [AuthenticatedActorType] is returned.
func AuditActorTypeValue(ctx context.Context) AuthenticatedActorType {
	ctxVal := ctx.Value(auditActorType{})
	val, is := ctxVal.(AuthenticatedActorType)
	if !is {
		return ""
	}
	return val
}

// AuditActorUUIDValue returns actor UUID recorded in authentication audit
// information on the context. If no value has been set on the context then a
// zero value string is returned.
func AuditActorUUIDValue(ctx context.Context) string {
	ctxVal := ctx.Value(auditActorUUID{})
	val, is := ctxVal.(string)
	if !is {
		return ""
	}
	return val
}

// AuditAuthenticatorNameValue returns the name of the authenticator used for
// authentication on the context. If no value has been set on the context then a
// zero value string is returned.
func AuditAuthenticatorNameValue(ctx context.Context) string {
	ctxVal := ctx.Value(auditAuthenticatorName{})
	val, is := ctxVal.(string)
	if !is {
		return ""
	}
	return val
}

// AuditAuthenticatorUsedValue returns the name of the authenticator used for
// authentication on the context. If no value has been set on the context then a
// zero value string is returned.
func AuditAuthenticatorUsedValue(ctx context.Context) string {
	ctxVal := ctx.Value(auditAuthenticatorUsed{})
	val, is := ctxVal.(string)
	if !is {
		return ""
	}
	return val
}

// WithAuditActorType returns a context that has authentication audit
// information applied stating the type of actor that has been authenticated.
func WithAuditActorType(
	ctx context.Context, actorType AuthenticatedActorType,
) context.Context {
	return context.WithValue(ctx, auditActorType{}, actorType)
}

// WithAuditActorUUID returns a context that has authentication audit
// information applied stating the UUID of the actor that has been
// authenticated.
func WithAuditActorUUID(ctx context.Context, actorUUID string) context.Context {
	return context.WithValue(ctx, auditActorUUID{}, actorUUID)
}

// WithAuditAuthenticatorName returns the name of the authenticator used for
// authentication of an actor.
func WithAuditAuthenticatorName(ctx context.Context, name string) context.Context {
	return context.WithValue(ctx, auditAuthenticatorName{}, name)
}

// WithAuditAuthenticatorUsed returns the type of authenticator used for
// authentication of an actor.
func WithAuditAuthenticatorUsed(
	ctx context.Context, authenticatorType string,
) context.Context {
	return context.WithValue(ctx, auditAuthenticatorUsed{}, authenticatorType)
}
