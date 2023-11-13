// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Machiner", 5, func(ctx facade.Context) (facade.Facade, error) {
		return newMachinerAPI(ctx) // Adds RecordAgentHostAndStartTime.
	}, reflect.TypeOf((*MachinerAPI)(nil)))
}

// newMachinerAPI creates a new instance of the Machiner API.
func newMachinerAPI(ctx facade.Context) (*MachinerAPI, error) {
	systemState, err := ctx.StatePool().SystemState()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return NewMachinerAPIForState(
		systemState,
		ctx.State(),
		ctx.Resources(),
		ctx.Auth(),
	)
}
