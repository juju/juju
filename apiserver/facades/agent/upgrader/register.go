// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package upgrader

import (
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/names/v4"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/state"
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
	registry.MustRegister("Upgrader", 1, func(ctx facade.Context) (facade.Facade, error) {
		return newUpgraderFacade(ctx)
	}, reflect.TypeOf((*Upgrader)(nil)).Elem())
}

// The upgrader facade is a bit unique vs the other API Facades, as it
// has two implementations that actually expose the same API and which
// one gets returned depends on who is calling.  Both of them conform
// to the exact Upgrader API, so the actual calls that are available
// do not depend on who is currently connected.

// newUpgraderFacade provides the signature required for facade registration.
func newUpgraderFacade(ctx facade.Context) (Upgrader, error) {
	auth := ctx.Auth()
	st := ctx.State()
	// The type of upgrader we return depends on who is asking.
	// Machines get an UpgraderAPI, units get a UnitUpgraderAPI.
	// This is tested in the api/upgrader package since there
	// are currently no direct srvRoot tests.
	// TODO(dfc) this is redundant
	tag, err := names.ParseTag(auth.GetAuthTag().String())
	if err != nil {
		return nil, apiservererrors.ErrPerm
	}
	model, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}

	ctrlSt := ctx.StatePool().SystemState()
	resources := ctx.Resources()
	switch tag.(type) {
	case names.MachineTag, names.ControllerAgentTag, names.ApplicationTag, names.ModelTag:
		return NewUpgraderAPI(ctrlSt, st, resources, auth)
	case names.UnitTag:
		if model.Type() == state.ModelTypeCAAS {
			// For sidecar applications.
			return NewUpgraderAPI(ctrlSt, st, resources, auth)
		}
		return NewUnitUpgraderAPI(ctx)
	}
	// Not a machine or unit.
	return nil, apiservererrors.ErrPerm
}
