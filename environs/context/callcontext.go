// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	stdcontext "context"

	"github.com/juju/errors"

	"github.com/juju/juju/domain/credential"
)

type (
	// InvalidateModelCredentialFunc records a credential as being invalid.
	InvalidateModelCredentialFunc func(reason string) error

	// InvalidateModelCredentialFuncGetter returns a function which records a credential as being invalid.
	InvalidateModelCredentialFuncGetter func() (InvalidateModelCredentialFunc, error)

	// CredentialIDFuncGetter returns a function which returns a credential ID.
	CredentialIDFuncGetter func() (credential.ID, error)

	// InvalidateCredentialFunc records a credential with the given ID as being invalid.
	InvalidateCredentialFunc func(ctx stdcontext.Context, id credential.ID, reason string) error
)

// ModelCredentialInvalidator defines a point of use interface for invalidating
// a model credential.
type ModelCredentialInvalidator interface {
	// InvalidateModelCredential invalidate cloud credential for the model.
	InvalidateModelCredential(string) error
}

// NewCredentialInvalidator creates a credential validator with
// callbacks which update dqlite and mongo.
func NewCredentialInvalidator(
	idGetter CredentialIDFuncGetter,
	invalidateFunc InvalidateCredentialFunc,
	legacyInvalidateFunc InvalidateModelCredentialFunc,
) ModelCredentialInvalidator {
	return &legacyCredentialAdaptor{
		idGetter:         idGetter,
		invalidateFunc:   invalidateFunc,
		legacyInvalidate: legacyInvalidateFunc,
	}
}

// legacyCredentialAdaptor exists as a *short term* solution to the fact that details
// for credential validity exists in both dqlite (on credential records) and mongo (on models).
// The provider calls a single InvalidateModelCredential function which updates both places.
type legacyCredentialAdaptor struct {
	idGetter         func() (credential.ID, error)
	invalidateFunc   func(ctx stdcontext.Context, id credential.ID, reason string) error
	legacyInvalidate func(string) error
}

// InvalidateModelCredential implements ModelCredentialInvalidator.
func (a *legacyCredentialAdaptor) InvalidateModelCredential(reason string) error {
	credId, err := a.idGetter()
	if err != nil {
		return errors.Trace(err)
	}
	if credId.IsZero() {
		return nil
	}
	if a.invalidateFunc == nil || a.legacyInvalidate == nil {
		return errors.New("missing validation functions")
	}
	if err := a.invalidateFunc(stdcontext.Background(), credId, reason); err != nil {
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
	stdcontext.Context
	invalidationFunc InvalidateModelCredentialFunc
}

// WithoutCredentialInvalidator returns a ProviderCallContext
// without any credential invalidation callback.
func WithoutCredentialInvalidator(ctx stdcontext.Context) ProviderCallContext {
	return ProviderCallContext{Context: ctx}
}

// WithCredentialInvalidator returns a ProviderCallContext
// with the specified credential invalidation callback.
func WithCredentialInvalidator(ctx stdcontext.Context, invalidationFunc InvalidateModelCredentialFunc) ProviderCallContext {
	return ProviderCallContext{
		Context:          ctx,
		invalidationFunc: invalidationFunc,
	}
}

// CredentialInvalidatorFromContext returns a credential invalidation func
// that may be associated with the context. If none, it returns a no-op function.
func CredentialInvalidatorFromContext(ctx ProviderCallContext) InvalidateModelCredentialFunc {
	if ctx.invalidationFunc != nil {
		return ctx.invalidationFunc
	}
	// No op.
	return func(string) error { return nil }
}
