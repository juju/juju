// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/juju/api/base"
	"github.com/juju/juju/api/credentialvalidator"
	"github.com/juju/juju/environs/context"
)

// CredentialAPI exposes functionality of the credential validator API facade to a worker.
type CredentialAPI interface {
	InvalidateModelCredential(reason string) error
}

// NewCredentialInvalidatorFacade creates an API facade capable of invalidating credential.
func NewCredentialInvalidatorFacade(apiCaller base.APICaller) (CredentialAPI, error) {
	return credentialvalidator.NewFacade(apiCaller), nil
}

// NewCloudCallContext creates a cloud call context to be used by workers.
func NewCloudCallContext(c CredentialAPI, dying context.Dying) context.ProviderCallContext {
	return &context.CloudCallContext{
		DyingFunc:                dying,
		InvalidateCredentialFunc: c.InvalidateModelCredential,
	}
}
