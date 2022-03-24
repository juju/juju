// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"reflect"

	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Machiner", 1, func(ctx facade.Context) (facade.Facade, error) {
		return newMachinerAPIV1(ctx)
	}, reflect.TypeOf((*MachinerAPIV1)(nil)))
	registry.MustRegister("Machiner", 2, func(ctx facade.Context) (facade.Facade, error) {
		return newMachinerAPIV2(ctx) // Adds RecordAgentStartTime.
	}, reflect.TypeOf((*MachinerAPIV2)(nil)))
	registry.MustRegister("Machiner", 3, func(ctx facade.Context) (facade.Facade, error) {
		return newMachinerAPIV3(ctx) // Relies on agent-set origin in SetObservedNetworkConfig.
	}, reflect.TypeOf((*MachinerAPIV3)(nil)))
	registry.MustRegister("Machiner", 4, func(ctx facade.Context) (facade.Facade, error) {
		return newMachinerAPIV4(ctx) // Removes SetProviderNetworkConfig.
	}, reflect.TypeOf((*MachinerAPIV4)(nil)))
	registry.MustRegister("Machiner", 5, func(ctx facade.Context) (facade.Facade, error) {
		return newMachinerAPI(ctx) // Adds RecordAgentHostAndStartTime.
	}, reflect.TypeOf((*MachinerAPI)(nil)))
}

// newMachinerAPIV1 creates a new instance of the V1 Machiner API.
func newMachinerAPIV1(
	ctx facade.Context,
) (*MachinerAPIV1, error) {
	api, err := newMachinerAPIV2(ctx)
	if err != nil {
		return nil, err
	}

	return &MachinerAPIV1{api}, nil
}

// newMachinerAPIV2 creates a new instance of the V2 Machiner API.
func newMachinerAPIV2(
	ctx facade.Context,
) (*MachinerAPIV2, error) {
	api, err := newMachinerAPIV3(ctx)
	if err != nil {
		return nil, err
	}

	return &MachinerAPIV2{api}, nil
}

// newMachinerAPIV3 creates a new instance of the V3 Machiner API.
func newMachinerAPIV3(
	ctx facade.Context,
) (*MachinerAPIV3, error) {
	api, err := newMachinerAPIV4(ctx)
	if err != nil {
		return nil, err
	}

	return &MachinerAPIV3{api}, nil
}

// newMachinerAPIV4 creates a new instance of the V4 Machiner API.
func newMachinerAPIV4(
	ctx facade.Context,
) (*MachinerAPIV4, error) {
	api, err := newMachinerAPI(ctx)
	if err != nil {
		return nil, err
	}

	return &MachinerAPIV4{api}, nil
}

// newMachinerAPI creates a new instance of the Machiner API.
func newMachinerAPI(ctx facade.Context) (*MachinerAPI, error) {
	return NewMachinerAPIForState(
		ctx.StatePool().SystemState(),
		ctx.State(),
		ctx.Resources(),
		ctx.Auth(),
	)
}
