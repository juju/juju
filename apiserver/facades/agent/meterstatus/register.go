// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package meterstatus

import (
	"reflect"

	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("MeterStatus", 1, func(ctx facade.Context) (facade.Facade, error) {
		return newMeterStatusFacadeV1(ctx)
	}, reflect.TypeOf((*MeterStatusAPIV1)(nil)))
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

// newMeterStatusFacadeV1 provides the signature required for the V1 facade registration.
func newMeterStatusFacadeV1(ctx facade.Context) (*MeterStatusAPIV1, error) {
	authorizer := ctx.Auth()
	resources := ctx.Resources()
	return NewMeterStatusAPIV1(ctx.State(), resources, authorizer)
}
