// Copyright 2022 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package applicationoffers

import (
	"context"
	"reflect"

	"github.com/juju/juju/apiserver/common"
	commoncrossmodel "github.com/juju/juju/apiserver/common/crossmodel"
	"github.com/juju/juju/apiserver/facade"
	"github.com/juju/names/v6"
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
		return makeOffersAPI(stdCtx, ctx)
	}, reflect.TypeOf((*OffersAPIv5)(nil)))
}

// makeOffersAPI returns a new application offers OffersAPI facade.
func makeOffersAPI(ctx context.Context, facadeContext facade.MultiModelContext) (*OffersAPIv5, error) {
	domainServices := facadeContext.DomainServices()
	st := facadeContext.State()
	getControllerInfo := func(ctx context.Context) ([]string, string, error) {
		return common.ControllerAPIInfo(ctx, st, domainServices.ControllerConfig())
	}

	authContext := facadeContext.Resources().Get("offerAccessAuthContext").(*common.ValueResource).Value

	controllerTag := names.NewControllerTag(facadeContext.ControllerUUID())

	return createOffersAPI(
		GetApplicationOffers,
		getControllerInfo,
		GetStateAccess(st),
		GetStatePool(facadeContext.StatePool()),
		domainServices.Access(),
		newModelDomainServicesGetter(facadeContext),
		facadeContext.Auth(),
		authContext.(*commoncrossmodel.AuthContext),
		facadeContext.DataDir(),
		facadeContext.Logger().Child("applicationoffers"),
		facadeContext.ModelUUID(),
		controllerTag,
		facadeContext.DomainServices().Model(),
	)
}
