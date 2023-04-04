// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machinemanager

import (
	"reflect"

	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("MachineManager", 9, func(ctx facade.Context) (facade.Facade, error) {
		return NewFacadeV9(ctx)
	}, reflect.TypeOf((*MachineManagerV9)(nil)))
	registry.MustRegister("MachineManager", 10, func(ctx facade.Context) (facade.Facade, error) {
		return NewFacadeV10(ctx) // DestroyMachineWithParams gains dry-run
	}, reflect.TypeOf((*MachineManagerAPI)(nil)))
}
