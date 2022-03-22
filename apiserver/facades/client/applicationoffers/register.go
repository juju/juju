// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package applicationoffers

import (
	"reflect"

	"github.com/juju/errors"
	"github.com/juju/juju/apiserver/common"
	commoncrossmodel "github.com/juju/juju/apiserver/common/crossmodel"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/juju/environs"
	"github.com/juju/juju/state/stateenvirons"
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
	registry.MustRegister("ApplicationOffers", 1, func(ctx facade.Context) (facade.Facade, error) {
		return newOffersAPI(ctx)
	}, reflect.TypeOf((*OffersAPI)(nil)))
	registry.MustRegister("ApplicationOffers", 2, func(ctx facade.Context) (facade.Facade, error) {
		return newOffersAPIV2(ctx)
	}, reflect.TypeOf((*OffersAPIV2)(nil)))
	registry.MustRegister("ApplicationOffers", 3, func(ctx facade.Context) (facade.Facade, error) {
		return newOffersAPIV3(ctx)
	}, reflect.TypeOf((*OffersAPIV3)(nil)))
	registry.MustRegister("ApplicationOffers", 4, func(ctx facade.Context) (facade.Facade, error) {
		return newOffersAPIV4(ctx)
	}, reflect.TypeOf((*OffersAPIV4)(nil)))
}

// newOffersAPI returns a new application offers OffersAPI facade.
func newOffersAPI(ctx facade.Context) (*OffersAPI, error) {
	environFromModel := func(modelUUID string) (environs.Environ, error) {
		st, err := ctx.StatePool().Get(modelUUID)
		if err != nil {
			return nil, errors.Trace(err)
		}
		defer st.Release()
		model, err := st.Model()
		if err != nil {
			return nil, errors.Trace(err)
		}
		g := stateenvirons.EnvironConfigGetter{Model: model}
		env, err := environs.GetEnviron(g, environs.New)
		if err != nil {
			return nil, errors.Trace(err)
		}
		return env, nil
	}

	st := ctx.State()
	getControllerInfo := func() ([]string, string, error) {
		return common.StateControllerInfo(st)
	}

	authContext := ctx.Resources().Get("offerAccessAuthContext").(common.ValueResource).Value
	return createOffersAPI(
		GetApplicationOffers,
		environFromModel,
		getControllerInfo,
		GetStateAccess(st),
		GetStatePool(ctx.StatePool()),
		ctx.Auth(),
		ctx.Resources(),
		authContext.(*commoncrossmodel.AuthContext),
	)
}

// newOffersAPIV2 returns a new application offers OffersAPIV2 facade.
func newOffersAPIV2(ctx facade.Context) (*OffersAPIV2, error) {
	apiV1, err := newOffersAPI(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &OffersAPIV2{OffersAPI: apiV1}, nil
}

// newOffersAPIV3 returns a new application offers OffersAPIV3 facade.
func newOffersAPIV3(ctx facade.Context) (*OffersAPIV3, error) {
	apiV2, err := newOffersAPIV2(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &OffersAPIV3{OffersAPIV2: apiV2}, nil
}

// newOffersAPIV4 returns a new application offers OffersAPIV4 facade.
func newOffersAPIV4(ctx facade.Context) (*OffersAPIV4, error) {
	apiV3, err := newOffersAPIV3(ctx)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &OffersAPIV4{OffersAPIV3: apiV3}, nil
}
