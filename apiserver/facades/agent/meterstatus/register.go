// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package meterstatus

import (
	"reflect"

	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/core/facades"
)

// FacadesVersions returns the versions of the facades that this package
// implements.
func FacadesVersions() facades.NamedFacadeVersion {
	return facades.NamedFacadeVersion{
		Name:     "MeterStatus",
		Versions: facades.FacadeVersion{2},
	}
}

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("MeterStatus", 2, func(ctx facade.Context) (facade.Facade, error) {
		return newMeterStatusFacade(ctx)
	}, reflect.TypeOf((*MeterStatusAPI)(nil)))
}

// newMeterStatusFacade provides the signature required for facade registration.
func newMeterStatusFacade(ctx facade.Context) (*MeterStatusAPI, error) {
	authorizer := ctx.Auth()
	resources := ctx.Resources()
	return NewMeterStatusAPI(ctx.State(), resources, authorizer)
}
