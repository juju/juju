// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package envcontext

import (
	"context"

	"github.com/juju/errors"

	"github.com/juju/juju/core/credential"
)

type (
	// ModelCredentialInvalidatorFunc records a credential as being invalid.
	ModelCredentialInvalidatorFunc func(ctx context.Context, reason string) error

	// CredentialKeyGetter is a function which returns a credential key.
	CredentialKeyGetter func() (credential.Key, error)

	// InvalidateCredentialFunc records a credential with the given key as being invalid.
	InvalidateCredentialFunc func(ctx context.Context, key credential.Key, reason string) error
)

// ModelCredentialInvalidator defines a point of use interface for invalidating
// a model credential.
type ModelCredentialInvalidator interface {
	// InvalidateModelCredential invalidate cloud credential for the model.
	InvalidateModelCredential(context.Context, string) error
}

// NewCredentialInvalidator creates a credential validator with
// callbacks which update dqlite and mongo.
func NewCredentialInvalidator(
	keyGetter CredentialKeyGetter,
	invalidateFunc InvalidateCredentialFunc,
	legacyInvalidateFunc func(reason string) error,
) ModelCredentialInvalidator {
	return &legacyCredentialAdaptor{
		keyGetter:        keyGetter,
		invalidateFunc:   invalidateFunc,
		legacyInvalidate: legacyInvalidateFunc,
	}
}

// legacyCredentialAdaptor exists as a *short term* solution to the fact that details
// for credential validity exists in both dqlite (on credential records) and mongo (on models).
// The provider calls a single InvalidateModelCredential function which updates both places.
type legacyCredentialAdaptor struct {
	keyGetter        func() (credential.Key, error)
	invalidateFunc   func(ctx context.Context, key credential.Key, reason string) error
	legacyInvalidate func(string) error
}

// InvalidateModelCredential implements ModelCredentialInvalidator.
func (a *legacyCredentialAdaptor) InvalidateModelCredential(ctx context.Context, reason string) error {
	credId, err := a.keyGetter()
	if err != nil {
		return errors.Trace(err)
	}
	if credId.IsZero() {
		return nil
	}
	if a.invalidateFunc == nil || a.legacyInvalidate == nil {
		return errors.New("missing validation functions")
	}
	if err := a.invalidateFunc(ctx, credId, reason); err != nil {
		return errors.Annotate(err, "updating credential details")
	}
	if err := a.legacyInvalidate(reason); err != nil {
		return errors.Annotate(err, "updating model credential details")
	}
	return nil
}

// ProviderCallContext wraps a standard context
// and is used in provider api calls.
type ProviderCallContext struct {
	context.Context
	invalidator ModelCredentialInvalidatorFunc
}

// WithoutCredentialInvalidator returns a ProviderCallContext
// without any credential invalidation callback.
func WithoutCredentialInvalidator(ctx context.Context) ProviderCallContext {
	return ProviderCallContext{Context: ctx}
}

// WithCredentialInvalidator returns a ProviderCallContext
// with the specified credential invalidation callback.
func WithCredentialInvalidator(ctx context.Context, invalidationFunc ModelCredentialInvalidatorFunc) ProviderCallContext {
	return ProviderCallContext{
		Context:     ctx,
		invalidator: invalidationFunc,
	}
}

// InvalidateCredential invalidates a credential with a reason.
func (ctx ProviderCallContext) InvalidateCredential(reason string) error {
	if ctx.invalidator != nil {
		return ctx.invalidator(ctx, reason)
	}
	return nil
}
