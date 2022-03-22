// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancepoller

import (
	"reflect"

	"github.com/juju/clock"
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
	registry.MustRegister("InstancePoller", 3, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV3(ctx)
	}, reflect.TypeOf((*InstancePollerAPIV3)(nil)))
	registry.MustRegister("InstancePoller", 4, func(ctx facade.Context) (facade.Facade, error) {
		return newFacade(ctx)
	}, reflect.TypeOf((*InstancePollerAPI)(nil)))
}

// newFacadeV3 creates a new instance of the V3 InstancePoller API.
func newFacadeV3(ctx facade.Context) (*InstancePollerAPIV3, error) {
	st := ctx.State()
	m, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}

	api, err := NewInstancePollerAPI(st, m, ctx.Resources(), ctx.Auth(), clock.WallClock)
	if err != nil {
		return nil, err
	}

	return &InstancePollerAPIV3{api}, nil
}

// newFacade wraps NewInstancePollerAPI for facade registration.
func newFacade(ctx facade.Context) (*InstancePollerAPI, error) {
	st := ctx.State()
	m, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return NewInstancePollerAPI(st, m, ctx.Resources(), ctx.Auth(), clock.WallClock)
}
