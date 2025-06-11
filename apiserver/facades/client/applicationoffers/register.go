// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package applicationoffers

import (
	"context"
	"reflect"

	"github.com/juju/errors"

	"github.com/juju/juju/apiserver/facade"
)

// Register is called to expose a package of facades onto a given registry.
func Register(registry facade.FacadeRegistry) {
	// Registering as a multi-model facade, to paper over the add-offer API.
	// This shouldn't be required, instead we should be in the context of a
	// model facade. Rather than rewriting this and the client for both 3.6
	// and 4.0, we're conceding. We lost.
	// This will have to be fixed and revisited in the future.
	// Note: to onlookers, this doesn't mean you should use this pattern
	// elsewhere. I've talked long and hard to myself about this, but there
	// is no way around it.
	registry.MustRegisterForMultiModel("ApplicationOffers", 5, func(stdCtx context.Context, ctx facade.MultiModelContext) (facade.Facade, error) {
		return makeOffersAPIV5(ctx)
	}, reflect.TypeOf((*OffersAPIv5)(nil)))
	registry.MustRegisterForMultiModel("ApplicationOffers", 6, func(stdCtx context.Context, ctx facade.MultiModelContext) (facade.Facade, error) {
		return makeOffersAPI(ctx)
	}, reflect.TypeOf((*OffersAPI)(nil)))
}

// makeOffersAPIv5 returns a new application offers OffersAPIv5 facade.
func makeOffersAPIV5(facadeContext facade.MultiModelContext) (*OffersAPIv5, error) {
	api, err := makeOffersAPI(facadeContext)
	if err != nil {
		return nil, errors.Trace(err)
	}
	return &OffersAPIv5{
		OffersAPI: api,
	}, nil
}

// makeOffersAPI returns a new application offers OffersAPI facade.
func makeOffersAPI(_ facade.MultiModelContext) (*OffersAPI, error) {
	return createOffersAPI()
}
