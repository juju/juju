// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import "github.com/juju/errors"

// NewCloudCallContext creates a new CloudCallContext to be used a
// ProviderCallContext.
func NewCloudCallContext() *CloudCallContext {
	return &CloudCallContext{
		InvalidateCredentialFunc: func(string) error {
			return errors.NotImplementedf("InvalidateCredentialCallback")
		},
	}
}

// CloudCallContext is a context intended to provide behaviors that are necessary
// to make a valid and lean call to an underlying substrate, for example cloud API.
//
// For instance, when Juju makes a call to cloud API with an expired credential,
// we might not yet know that it is expired until cloud API rejects it. However,
// we do know in advance, before making the call, that we want to mark this
// credential as invalid if the cloud API rejects it.
// How credential will be found, where it is stored in Juju data model,
// what calls need to be done to mark it so,
// will be the responsibility of internal functions that are passed in to this context
// as this knowledge is specific to where the call was made *from* not on what object
// it was made.
type CloudCallContext struct {
	// InvalidateCredentialFunc is the actual callback function
	// that invalidates the credential used in the context of this call.
	InvalidateCredentialFunc func(string) error

	// DyingFunc returns the dying chan.
	DyingFunc Dying
}

// InvalidateCredential implements context.InvalidateCredentialCallback.
func (c *CloudCallContext) InvalidateCredential(reason string) error {
	return c.InvalidateCredentialFunc(reason)
}

// Dying returns the dying chan.
func (c *CloudCallContext) Dying() <-chan struct{} {
	if c.DyingFunc == nil {
		return nil
	}
	return c.DyingFunc()
}
