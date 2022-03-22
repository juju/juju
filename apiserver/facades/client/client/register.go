// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

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
	registry.MustRegister("Client", 1, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV1(ctx)
	}, reflect.TypeOf((*ClientV1)(nil)))
	registry.MustRegister("Client", 2, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV2(ctx)
	}, reflect.TypeOf((*ClientV2)(nil)))
	registry.MustRegister("Client", 3, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV3(ctx)
	}, reflect.TypeOf((*ClientV3)(nil)))
	registry.MustRegister("Client", 4, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV4(ctx)
	}, reflect.TypeOf((*ClientV4)(nil)))
	registry.MustRegister("Client", 5, func(ctx facade.Context) (facade.Facade, error) {
		return NewFacade(ctx)
	}, reflect.TypeOf((*Client)(nil)))
}

// newFacadeV1 creates a version 1 Client facade to handle API requests.
func newFacadeV1(ctx facade.Context) (*ClientV1, error) {
	client, err := newFacadeV2(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &ClientV1{client}, nil
}

// newFacadeV2 creates a version 2 Client facade to handle API requests.
func newFacadeV2(ctx facade.Context) (*ClientV2, error) {
	client, err := newFacadeV3(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &ClientV2{client}, nil
}

// newFacadeV3 creates a version 3 Client facade to handle API requests.
func newFacadeV3(ctx facade.Context) (*ClientV3, error) {
	client, err := newFacadeV4(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &ClientV3{client}, nil
}

// newFacadeV4 creates a version 4 Client facade to handle API requests.
func newFacadeV4(ctx facade.Context) (*ClientV4, error) {
	client, err := NewFacade(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &ClientV4{client}, nil
}
