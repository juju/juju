// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialvalidator

import (
	"reflect"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/facades"
)

// FacadesVersions returns the versions of the facades that this package
// implements.
func FacadesVersions() facades.NamedFacadeVersion {
	return facades.NamedFacadeVersion{
		Name:     "CredentialValidator",
		Versions: facades.FacadeVersion{2},
	}
}

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("CredentialValidator", 2, func(ctx facade.Context) (facade.Facade, error) {
		return newCredentialValidatorAPI(ctx) // adds WatchModelCredential
	}, reflect.TypeOf((*CredentialValidatorAPI)(nil)))
}

// newCredentialValidatorAPI creates a new CredentialValidator API endpoint on server-side.
func newCredentialValidatorAPI(ctx facade.Context) (*CredentialValidatorAPI, error) {
	return internalNewCredentialValidatorAPI(NewBackend(NewStateShim(ctx.State())), ctx.Resources(), ctx.Auth())
}
