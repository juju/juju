// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

import (
	"reflect"

	"github.com/juju/errors"

	apiservererrors "github.com/juju/juju/apiserver/errors"
	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	registry.MustRegister("Client", 6, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV6(ctx)
	}, reflect.TypeOf((*ClientV6)(nil)))
	registry.MustRegister("Client", 7, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV7(ctx)
	}, reflect.TypeOf((*ClientV7)(nil)))
	registry.MustRegister("Client", 8, func(ctx facade.Context) (facade.Facade, error) {
		return newFacadeV8(ctx)
	}, reflect.TypeOf((*Client)(nil)))
}

// newFacadeV6 returns a new ClientV6 facade.
func newFacadeV6(ctx facade.Context) (*ClientV6, error) {
	client, err := newFacadeV7(ctx)
	if err != nil {
		return nil, err
	}
	return &ClientV6{client}, nil
}

// newFacadeV7 returns a new ClientV7 facade.
func newFacadeV7(ctx facade.Context) (*ClientV7, error) {
	return NewFacadeV7(ctx)
}

// newFacadeV8 returns a new Client facade (v8).
func newFacadeV8(ctx facade.Context) (*Client, error) {
	authorizer := ctx.Auth()
	if !authorizer.AuthClient() {
		return nil, apiservererrors.ErrPerm
	}

	st := ctx.State()
	resources := ctx.Resources()
	presence := ctx.Presence()
	factory := ctx.MultiwatcherFactory()

	model, err := st.Model()
	if err != nil {
		return nil, errors.Trace(err)
	}
	modelUUID := model.UUID()

	leadershipReader, err := ctx.LeadershipReader(modelUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}

	modelCache, err := ctx.CachedModel(modelUUID)
	if err != nil {
		return nil, errors.Trace(err)
	}

	storageAccessor, err := getStorageState(st)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return &Client{
		stateAccessor:       &stateShim{st, model, nil},
		storageAccessor:     storageAccessor,
		auth:                authorizer,
		presence:            presence,
		leadershipReader:    leadershipReader,
		modelCache:          modelCache,
		resources:           resources,
		multiwatcherFactory: factory,
	}, nil
}
