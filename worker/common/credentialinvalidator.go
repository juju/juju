// Copyright 2018 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package common

import (
	"github.com/juju/errors"

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
	f := credentialvalidator.NewFacade(apiCaller)
	if f == nil {
		return nil, errors.New("could not create credential validator facade")
	}
	return f, nil
}

func NewCloudCallContext(c CredentialAPI) context.ProviderCallContext {
	return &context.CloudCallContext{
		InvalidateCredentialF: c.InvalidateModelCredential,
	}
}
