// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package resourceshookcontext

import (
	"context"
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/names/v5"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/state"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("ResourcesHookContext", 1, func(stdCtx context.Context, ctx facade.ModelContext) (facade.Facade, error) {
		return newStateFacade(ctx)
	}, reflect.TypeOf((*UnitFacade)(nil)))
}

// newStateFacade provides the signature to register this resource facade
func newStateFacade(ctx facade.ModelContext) (*UnitFacade, error) {
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

	res := st.Resources(ctx.ObjectStore())
	return NewUnitFacade(&resourcesUnitDataStore{res, unit}), nil
}
