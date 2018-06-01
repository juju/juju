// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package context

import "github.com/juju/errors"

// ProviderCallContext exposes useful capabilities when making calls
// to an underlying cloud substrate.
type ProviderCallContext interface {

	// InvalidateCredential provides means to invalidate a credential
	// that is used to make a call.
	InvalidateCredential(string) error
}

func NewCloudCallContext() *CloudCallContext {
	return &CloudCallContext{
		InvalidateCredentialF: func(string) error {
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
	InvalidateCredentialF func(string) error
}

// InvalidateCredentialCallback implements context.InvalidateCredentialCallback.
func (c *CloudCallContext) InvalidateCredential(reason string) error {
	return c.InvalidateCredentialF(reason)
}
