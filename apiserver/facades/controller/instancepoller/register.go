// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package instancepoller

import (
	"reflect"

	"github.com/juju/clock"
	"github.com/juju/errors"
	"github.com/juju/juju/v2/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("InstancePoller", 3, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV3(ctx)
	}, reflect.TypeOf((*InstancePollerAPIV3)(nil)))
	registry.MustRegister("InstancePoller", 4, func(ctx facade.Context) (facade.Facade, error) {
		return newFacade(ctx)
	}, reflect.TypeOf((*InstancePollerAPI)(nil)))
}

// newFacadeV3 creates a new instance of the V3 InstancePoller API.
func newFacadeV3(ctx facade.Context) (*InstancePollerAPIV3, error) {
	st := ctx.State()
	m, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}

	api, err := NewInstancePollerAPI(st, m, ctx.Resources(), ctx.Auth(), clock.WallClock)
	if err != nil {
		return nil, err
	}

	return &InstancePollerAPIV3{api}, nil
}

// newFacade wraps NewInstancePollerAPI for facade registration.
func newFacade(ctx facade.Context) (*InstancePollerAPI, error) {
	st := ctx.State()
	m, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	return NewInstancePollerAPI(st, m, ctx.Resources(), ctx.Auth(), clock.WallClock)
}
