// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialvalidator

import (
	"reflect"

	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("CredentialValidator", 1, func(ctx facade.Context) (facade.Facade, error) {
		return newCredentialValidatorAPIv1(ctx)
	}, reflect.TypeOf((*CredentialValidatorAPIV1)(nil)))
	registry.MustRegister("CredentialValidator", 2, func(ctx facade.Context) (facade.Facade, error) {
		return newCredentialValidatorAPI(ctx) // adds WatchModelCredential
	}, reflect.TypeOf((*CredentialValidatorAPI)(nil)))
}

// newCredentialValidatorAPIv1 creates a new CredentialValidator API endpoint on server-side.
func newCredentialValidatorAPIv1(ctx facade.Context) (*CredentialValidatorAPIV1, error) {
	v2, err := newCredentialValidatorAPI(ctx)
	if err != nil {
		return nil, err
	}
	return &CredentialValidatorAPIV1{v2}, nil
}

// newCredentialValidatorAPI creates a new CredentialValidator API endpoint on server-side.
func newCredentialValidatorAPI(ctx facade.Context) (*CredentialValidatorAPI, error) {
	return internalNewCredentialValidatorAPI(NewBackend(NewStateShim(ctx.State())), ctx.Resources(), ctx.Auth())
}
