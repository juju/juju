// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package modelgeneration

import (
	"reflect"

	"github.com/juju/errors"
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
	registry.MustRegister("ModelGeneration", 1, func(ctx facade.Context) (facade.Facade, error) {
		return newModelGenerationFacade(ctx)
	}, reflect.TypeOf((*APIV1)(nil)))
	registry.MustRegister("ModelGeneration", 2, func(ctx facade.Context) (facade.Facade, error) {
		return newModelGenerationFacadeV2(ctx)
	}, reflect.TypeOf((*APIV1)(nil)))
	registry.MustRegister("ModelGeneration", 3, func(ctx facade.Context) (facade.Facade, error) {
		return newModelGenerationFacadeV3(ctx)
	}, reflect.TypeOf((*APIV1)(nil)))
	registry.MustRegister("ModelGeneration", 4, func(ctx facade.Context) (facade.Facade, error) {
		return newModelGenerationFacadeV4(ctx)
	}, reflect.TypeOf((*APIV1)(nil)))
}

// newModelGenerationFacadeV4 provides the signature required for facade registration.
func newModelGenerationFacadeV4(ctx facade.Context) (*API, error) {
	authorizer := ctx.Auth()
	st := &stateShim{State: ctx.State()}
	m, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	mc, err := ctx.Controller().Model(st.ModelUUID())
	if err != nil {
		return nil, errors.Trace(err)
	}
	return NewModelGenerationAPI(st, authorizer, m, &modelCacheShim{Model: mc})
}

// newModelGenerationFacadeV3 provides the signature required for facade registration.
func newModelGenerationFacadeV3(ctx facade.Context) (*APIV3, error) {
	v4, err := newModelGenerationFacadeV4(ctx)
	if err != nil {
		return nil, err
	}
	return &APIV3{v4}, nil

} // newModelGenerationFacadeV2 provides the signature required for facade registration.
func newModelGenerationFacadeV2(ctx facade.Context) (*APIV2, error) {
	v3, err := newModelGenerationFacadeV3(ctx)
	if err != nil {
		return nil, err
	}
	return &APIV2{v3}, nil
}

// newModelGenerationFacade provides the signature required for facade registration.
func newModelGenerationFacade(ctx facade.Context) (*APIV1, error) {
	v2, err := newModelGenerationFacadeV2(ctx)
	if err != nil {
		return nil, err
	}
	return &APIV1{v2}, nil
}
