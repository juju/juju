// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import (
	stdcontext "context"

	"github.com/juju/errors"
	"github.com/juju/names/v4"

	"github.com/juju/juju/domain/credential"
)

// ModelCredentialInvalidator defines a point of use interface for invalidating
// a model credential.
type ModelCredentialInvalidator interface {
	// InvalidateModelCredential invalidate cloud credential for the model.
	InvalidateModelCredential(string) error
}

// NewCredentialInvalidator returns an instance which is used by providers to
// mark a credential as invalid.
func NewCredentialInvalidator(
	tag names.CloudCredentialTag,
	invalidateFunc func(ctx stdcontext.Context, id credential.ID, reason string) error,
	legacyInvalidate func(reason string) error,
) ModelCredentialInvalidator {
	return &legacyCredentialAdaptor{
		tag:              tag,
		invalidateFunc:   invalidateFunc,
		legacyInvalidate: legacyInvalidate,
	}
}

// legacyCredentialAdaptor exists as a *short term* solution to the fact that details
// for credential validity exists in both dqlite (on credential records) and mongo (on models).
// The provider calls a single InvalidateModelCredential function which updates both places.
// TODO(wallyworld) - the legacy invalidate function needs to be changed to take a context and tag.
type legacyCredentialAdaptor struct {
	tag              names.CloudCredentialTag
	invalidateFunc   func(ctx stdcontext.Context, id credential.ID, reason string) error
	legacyInvalidate func(string) error
}

// InvalidateModelCredential implements ModelCredentialInvalidator.
func (a *legacyCredentialAdaptor) InvalidateModelCredential(reason string) error {
	if a.tag.IsZero() {
		return errors.New("missing credential tag")
	}
	if a.invalidateFunc == nil || a.legacyInvalidate == nil {
		return errors.New("missing validation functions")
	}
	if err := a.invalidateFunc(stdcontext.Background(), credential.IdFromTag(a.tag), reason); err != nil {
		return errors.Annotate(err, "updating credential details")
	}
	if err := a.legacyInvalidate(reason); err != nil {
		return errors.Annotate(err, "updating model credential details")
	}
	return nil
}

// CallContext creates a CloudCallContext for use when calling environ methods
// that may require invalidate a cloud credential.
func CallContext(credential ModelCredentialInvalidator) *CloudCallContext {
	// TODO(wallyworld) - pass in the stdcontext
	callCtx := NewCloudCallContext(stdcontext.Background())
	callCtx.InvalidateCredentialFunc = credential.InvalidateModelCredential
	return callCtx
}
