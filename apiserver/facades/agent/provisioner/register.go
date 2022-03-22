// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provisioner

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
	registry.MustRegister("Provisioner", 3, func(ctx facade.Context) (facade.Facade, error) {
		return newProvisionerAPIV4(ctx) // Yes this is weird.
	}, reflect.TypeOf((*ProvisionerAPIV4)(nil)))
	registry.MustRegister("Provisioner", 4, func(ctx facade.Context) (facade.Facade, error) {
		return newProvisionerAPIV4(ctx)
	}, reflect.TypeOf((*ProvisionerAPIV4)(nil)))
	registry.MustRegister("Provisioner", 5, func(ctx facade.Context) (facade.Facade, error) {
		return newProvisionerAPIV5(ctx) // Adds DistributionGroupByMachineId()
	}, reflect.TypeOf((*ProvisionerAPIV5)(nil)))
	registry.MustRegister("Provisioner", 6, func(ctx facade.Context) (facade.Facade, error) {
		return newProvisionerAPIV6(ctx) // Adds more proxy settings
	}, reflect.TypeOf((*ProvisionerAPIV6)(nil)))
	registry.MustRegister("Provisioner", 7, func(ctx facade.Context) (facade.Facade, error) {
		return newProvisionerAPIV7(ctx) // Adds charm profile watcher
	}, reflect.TypeOf((*ProvisionerAPIV7)(nil)))
	registry.MustRegister("Provisioner", 8, func(ctx facade.Context) (facade.Facade, error) {
		return newProvisionerAPIV8(ctx) // Adds changes charm profile and modification status
	}, reflect.TypeOf((*ProvisionerAPIV8)(nil)))
	registry.MustRegister("Provisioner", 9, func(ctx facade.Context) (facade.Facade, error) {
		return newProvisionerAPIV9(ctx) // Adds supported containers
	}, reflect.TypeOf((*ProvisionerAPIV9)(nil)))
	registry.MustRegister("Provisioner", 10, func(ctx facade.Context) (facade.Facade, error) {
		return newProvisionerAPIV10(ctx) // Adds support for multiple space constraints.
	}, reflect.TypeOf((*ProvisionerAPIV10)(nil)))
	registry.MustRegister("Provisioner", 11, func(ctx facade.Context) (facade.Facade, error) {
		return newProvisionerAPIV11(ctx) // Relies on agent-set origin in SetHostMachineNetworkConfig.
	}, reflect.TypeOf((*ProvisionerAPIV11)(nil)))
}

// newProvisionerAPIV4 creates a new server-side version 4 Provisioner API facade.
func newProvisionerAPIV4(ctx facade.Context) (*ProvisionerAPIV4, error) {
	provisionerAPI, err := newProvisionerAPIV5(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &ProvisionerAPIV4{provisionerAPI}, nil
}

// newProvisionerAPIV5 creates a new server-side Provisioner API facade.
func newProvisionerAPIV5(ctx facade.Context) (*ProvisionerAPIV5, error) {
	provisionerAPI, err := newProvisionerAPIV6(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &ProvisionerAPIV5{provisionerAPI}, nil
}

// newProvisionerAPIV6 creates a new server-side Provisioner API facade.
func newProvisionerAPIV6(ctx facade.Context) (*ProvisionerAPIV6, error) {
	provisionerAPI, err := newProvisionerAPIV7(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &ProvisionerAPIV6{provisionerAPI}, nil
}

// newProvisionerAPIV7 creates a new server-side Provisioner API facade.
func newProvisionerAPIV7(ctx facade.Context) (*ProvisionerAPIV7, error) {
	provisionerAPI, err := newProvisionerAPIV8(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &ProvisionerAPIV7{provisionerAPI}, nil
}

// newProvisionerAPIV8 creates a new server-side Provisioner API facade.
func newProvisionerAPIV8(ctx facade.Context) (*ProvisionerAPIV8, error) {
	provisionerAPI, err := newProvisionerAPIV9(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &ProvisionerAPIV8{provisionerAPI}, nil
}

// newProvisionerAPIV9 creates a new server-side Provisioner API facade.
func newProvisionerAPIV9(ctx facade.Context) (*ProvisionerAPIV9, error) {
	provisionerAPI, err := newProvisionerAPIV10(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &ProvisionerAPIV9{provisionerAPI}, nil
}

// newProvisionerAPIV10 creates a new server-side Provisioner API facade.
func newProvisionerAPIV10(ctx facade.Context) (*ProvisionerAPIV10, error) {
	provisionerAPI, err := newProvisionerAPIV11(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &ProvisionerAPIV10{provisionerAPI}, nil
}

// newProvisionerAPIV11 creates a new server-side Provisioner API facade.
func newProvisionerAPIV11(ctx facade.Context) (*ProvisionerAPIV11, error) {
	provisionerAPI, err := NewProvisionerAPI(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &ProvisionerAPIV11{provisionerAPI}, nil
}
