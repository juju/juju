// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machinemanager

import (
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/v3/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("MachineManager", 7, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV7(ctx)
	}, reflect.TypeOf((*MachineManagerAPI)(nil)))
}

// newFacadeV7 creates a new server-side MachineManager API facade.
func newFacadeV7(ctx facade.Context) (*MachineManagerAPI, error) {
	machineManagerAPI, err := NewFacadeV7(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return machineManagerAPI, nil
}
