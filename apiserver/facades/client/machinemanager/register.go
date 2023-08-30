// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machinemanager

import (
	"reflect"

	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("MachineManager", 11, func(ctx facade.Context) (facade.Facade, error) {
		return NewFacadeV11(ctx) // DestroyMachineWithParams gains dry-run
	}, reflect.TypeOf((*MachineManagerAPI)(nil)))
}
