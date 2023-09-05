// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialvalidator

import (
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("CredentialValidator", 2, func(ctx facade.Context) (facade.Facade, error) {
		return newCredentialValidatorAPI(ctx) // adds WatchModelCredential
	}, reflect.TypeOf((*CredentialValidatorAPI)(nil)))
}

// newCredentialValidatorAPI creates a new CredentialValidator API endpoint on server-side.
func newCredentialValidatorAPI(ctx facade.Context) (*CredentialValidatorAPI, error) {
	st, err := NewStateShim(ctx.State())
	if err != nil {
		return nil, errors.Trace(err)
	}
	return internalNewCredentialValidatorAPI(st,
		ctx.ServiceFactory().Credential(),
		ctx.Resources(), ctx.Auth(),
		ctx.Logger().Child("credentialvalidator"))
}
