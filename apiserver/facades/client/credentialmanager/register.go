// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package credentialmanager

import (
	"reflect"

	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("CredentialManager", 1, func(ctx facade.Context) (facade.Facade, error) {
		return newCredentialManagerAPI(ctx)
	}, reflect.TypeOf((*CredentialManagerAPI)(nil)))
}

// newCredentialManagerAPI creates a new CredentialManager API endpoint on server-side.
func newCredentialManagerAPI(ctx facade.Context) (*CredentialManagerAPI, error) {
	return internalNewCredentialManagerAPI(newStateShim(ctx.State()), ctx.Resources(), ctx.Auth())
}
