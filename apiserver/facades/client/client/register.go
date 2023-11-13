// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"reflect"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/facades"
)

// FacadesVersions returns the versions of the facades that this package
// implements.
func FacadesVersions() facades.NamedFacadeVersion {
	return facades.NamedFacadeVersion{
		Name:     "Client",
		Versions: facades.FacadeVersion{6},
	}
}

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Client", 6, func(ctx facade.Context) (facade.Facade, error) {
		return NewFacade(ctx)
	}, reflect.TypeOf((*Client)(nil)))
}
