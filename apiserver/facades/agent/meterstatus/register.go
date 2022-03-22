// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package meterstatus

import (
	"reflect"

	"github.com/juju/juju/apiserver/facade"
)

// Registry describes the API facades exposed by some API server.
type Registry interface {
	// MustRegister adds a single named facade at a given version to the
	// registry.
	// Factory will be called when someone wants to instantiate an object of
	// this facade, and facadeType defines the concrete type that the returned
	// object will be.
	// The Type information is used to define what methods will be exported in
	// the API, and it must exactly match the actual object returned by the
	// factory.
	MustRegister(string, int, facade.Factory, reflect.Type)
}

// Register is called to expose a package of facades onto a given registry.
func Register(registry Registry) {
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
