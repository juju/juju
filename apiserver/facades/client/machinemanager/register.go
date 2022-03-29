// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machinemanager

import (
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("MachineManager", 2, func(ctx facade.Context) (facade.Facade, error) {
		return newFacade(ctx)
	}, reflect.TypeOf((*MachineManagerAPI)(nil)))
	registry.MustRegister("MachineManager", 3, func(ctx facade.Context) (facade.Facade, error) {
		return newFacade(ctx) // Adds DestroyMachine and ForceDestroyMachine.
	}, reflect.TypeOf((*MachineManagerAPI)(nil)))
	registry.MustRegister("MachineManager", 4, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV4(ctx) // Adds DestroyMachineWithParams.
	}, reflect.TypeOf((*MachineManagerAPIV4)(nil)))
	registry.MustRegister("MachineManager", 5, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV5(ctx) // Adds UpgradeSeriesPrepare, removes UpdateMachineSeries.
	}, reflect.TypeOf((*MachineManagerAPIV5)(nil)))
	registry.MustRegister("MachineManager", 6, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV6(ctx) // DestroyMachinesWithParams gains maxWait.
	}, reflect.TypeOf((*MachineManagerAPIV6)(nil)))
}

// newFacade creates a new server-side MachineManager API facade.
func newFacade(ctx facade.Context) (*MachineManagerAPI, error) {
	return NewFacade(ctx)
}

// newFacadeV4 creates a new server-side MachineManager API facade.
func newFacadeV4(ctx facade.Context) (*MachineManagerAPIV4, error) {
	machineManagerAPIV5, err := newFacadeV5(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &MachineManagerAPIV4{machineManagerAPIV5}, nil
}

// newFacadeV5 creates a new server-side MachineManager API facade.
func newFacadeV5(ctx facade.Context) (*MachineManagerAPIV5, error) {
	machineManagerAPIv6, err := newFacadeV6(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &MachineManagerAPIV5{machineManagerAPIv6}, nil
}

// newFacadeV6 creates a new server-side MachineManager API facade.
func newFacadeV6(ctx facade.Context) (*MachineManagerAPIV6, error) {
	machineManagerAPI, err := newFacade(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &MachineManagerAPIV6{machineManagerAPI}, nil
}
