// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package machine

import (
	"context"
	"reflect"

	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	// Register the Machiner facade at version 6, which relies on the dqlite
	// backend. SetMachineAddresses is removed (to be handled by the network
	// api).
	registry.MustRegister("Machiner", 6, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newMachinerAPI(stdCtx, ctx)
	}, reflect.TypeOf((*MachinerAPI)(nil)))
	// Register the Machiner facade at version 5, which, on Juju 4.0, stubs out
	// the Jobs() and SetMachineAddresses() methods.
	registry.MustRegister("Machiner", 5, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newMachinerAPIV5(stdCtx, ctx) // Adds RecordAgentHostAndStartTime.
	}, reflect.TypeOf((*MachinerAPIv5)(nil)))
}

// newMachinerAPI creates a new instance of the Machiner API.
func newMachinerAPI(stdCtx context.Context, ctx facade.ModelContext) (*MachinerAPI, error) {
	domainServices := ctx.DomainServices()
	return NewMachinerAPIForState(
		stdCtx,
		ctx.State(),
		ctx.Clock(),
		domainServices.ControllerConfig(),
		domainServices.ControllerNode(),
		domainServices.ModelInfo(),
		domainServices.Network(),
		domainServices.Application(),
		domainServices.Machine(),
		domainServices.Status(),
		ctx.WatcherRegistry(),
		ctx.Auth(),
		ctx.Logger(),
	)
}

// newMachinerAPIV5 creates a new instance of the Machiner API at version 5.
func newMachinerAPIV5(stdCtx context.Context, ctx facade.ModelContext) (*MachinerAPIv5, error) {
	api, err := newMachinerAPI(stdCtx, ctx)
	if err != nil {
		return nil, err
	}
	return &MachinerAPIv5{
		MachinerAPI: api,
	}, nil
}
