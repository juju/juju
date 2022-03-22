// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resourceshookcontext

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
	registry.MustRegister("ResourcesHookContext", 1, func(ctx facade.Context) (facade.Facade, error) {
		return newStateFacade(ctx)
	}, reflect.TypeOf((*UnitFacade)(nil)))
}

// newStateFacade provides the signature to register this resource facade
func newStateFacade(ctx facade.Context) (*UnitFacade, error) {
	authorizer := ctx.Auth()
	st := ctx.State()

	if !authorizer.AuthUnitAgent() && !authorizer.AuthApplicationAgent() {
		return nil, apiservererrors.ErrPerm
	}

	var (
		unit *state.Unit
		err  error
	)
	switch tag := authorizer.GetAuthTag().(type) {
	case names.UnitTag:
		unit, err = st.Unit(tag.Id())
		if err != nil {
			return nil, errors.Trace(err)
		}
	case names.ApplicationTag:
		// Allow application access for K8s units. As they are all homogeneous any of the units will suffice.
		app, err := st.Application(tag.Id())
		if err != nil {
			return nil, errors.Trace(err)
		}
		allUnits, err := app.AllUnits()
		if err != nil {
			return nil, errors.Trace(err)
		}
		if len(allUnits) <= 0 {
			return nil, errors.Errorf("failed to get units for app: %s", app.Name())
		}
		unit = allUnits[0]
	default:
		return nil, errors.Errorf("expected names.UnitTag or names.ApplicationTag, got %T", tag)
	}

	res, err := st.Resources()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return NewUnitFacade(&resourcesUnitDataStore{res, unit}), nil
}
