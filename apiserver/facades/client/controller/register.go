// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package controller

import (
	"fmt"
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Controller", 11, func(ctx facade.Context) (facade.Facade, error) {
		api, err := makeControllerAPIv11(ctx)
		if err != nil {
			return nil, fmt.Errorf("creating Controller facade v11: %w", err)
		}
		return api, nil
	}, reflect.TypeOf((*ControllerAPIv11)(nil)))

	registry.MustRegister("Controller", 12, func(ctx facade.Context) (facade.Facade, error) {
		api, err := makeControllerAPI(ctx)
		if err != nil {
			return nil, fmt.Errorf("creating Controller facade v12: %w", err)
		}
		return api, nil
	}, reflect.TypeOf((*ControllerAPI)(nil)))
}

func makeControllerAPI(ctx facade.Context) (*ControllerAPI, error) {
	st := ctx.State()
	authorizer := ctx.Auth()
	pool := ctx.StatePool()
	resources := ctx.Resources()
	presence := ctx.Presence()
	hub := ctx.Hub()
	factory := ctx.MultiwatcherFactory()
	controller := ctx.Controller()

	leadership, err := ctx.LeadershipReader(st.ModelUUID())
	if err != nil {
		return nil, errors.Trace(err)
	}

	return NewControllerAPI(
		st,
		pool,
		authorizer,
		resources,
		presence,
		hub,
		factory,
		controller,
		leadership,
	)
}

// makeControllerAPIv11 creates a new ControllerAPIv11
func makeControllerAPIv11(ctx facade.Context) (*ControllerAPIv11, error) {
	controllerAPI, err := makeControllerAPI(ctx)
	if err != nil {
		return nil, err
	}

	return &ControllerAPIv11{
		ControllerAPI: controllerAPI,
	}, nil
}
